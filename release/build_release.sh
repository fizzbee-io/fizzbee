#!/bin/bash

set -e  # Exit on error

SCRIPT_DIR="$(dirname "$(readlink -f "${BASH_SOURCE[0]:-$0}")")"
PROJECT_DIR="$(dirname $SCRIPT_DIR)"

# Get the current date for versioning if FIZZBEE_RELEASE_VERSION is not set
VERSION="${FIZZBEE_RELEASE_VERSION:-$(date +%Y%m%d)}"
RELEASE_DIR="fizzbee-$VERSION"
mkdir -p releases

# The platform this script is running on, in PLATFORMS naming. Cross-compiled
# artifacts cannot be executed here, so the end-to-end smoke test below runs
# only for the native platform (linux_x86 on the release CI runner).
HOST_PLATFORM=""
case "$(uname -s)-$(uname -m)" in
    Linux-x86_64)  HOST_PLATFORM=linux_x86 ;;
    Linux-aarch64) HOST_PLATFORM=linux_arm ;;
    Darwin-arm64)  HOST_PLATFORM=macos_arm ;;
    Darwin-x86_64) HOST_PLATFORM=macos_x86 ;;
esac

# TODO: Couldn't build for windows yet, and the bash script has to be converted to BAT or something else.

# Define platforms
PLATFORMS=(
    "macos_x86"
    "macos_arm"
    "linux_x86"
    "linux_arm"
#    "windows_x86"
#    "windows_arm"
)

# Build and package for each platform
for PLATFORM in "${PLATFORMS[@]}"; do
    echo "Building for $PLATFORM..."

    # Run Bazel build
    bazel build --platforms=//:"$PLATFORM" //parser/... //:fizzbee //mbt/generator:generator_bin_zip

    # Create target directory
    TARGET_DIR="$RELEASE_DIR-$PLATFORM"
    mkdir -p "$TARGET_DIR"

    # Copy files
    cp -L -R bazel-bin/parser "$TARGET_DIR"
    cp bazel-bin/fizzbee_/fizzbee "$TARGET_DIR"
    cp bazel-bin/mbt/generator/generator_bin.zip "$TARGET_DIR/mbt_gen.zip"

    # Strip pip and ensurepip from the bundled hermetic python toolchain.
    # The parser never uses them at runtime, and the stale bundled pip is
    # the only CVE-scanner finding the release tarball itself owns (e.g.
    # CVE-2025-8869 against pip 24.3.1 in the rules_python toolchain;
    # ensurepip embeds another copy of the same pip as a wheel). Removing
    # them clears the findings and shrinks the tarball.
    #
    # Name patterns are deliberately exact: the pip-INSTALLED dependency
    # repos (rules_python++pip+pypi_312_antlr4_python3_runtime, protobuf)
    # merely contain "pip" in their names and must survive. bin/pip3*
    # (rather than a hardcoded pip3.12) keeps the strip working across
    # future python minor-version bumps.
    chmod -R u+w "$TARGET_DIR/parser"
    find "$TARGET_DIR/parser" -type d \( -name "pip" -o -name "pip-*.dist-info" -o -name "ensurepip" \) -prune -exec rm -rf {} +
    find "$TARGET_DIR/parser" -type f \( -name "pip" -o -name "pip3*" \) -path "*/bin/*" -delete

    # Packaging invariants — fail the release loudly rather than publish a
    # tarball that violates them.
    # 1. Negative: nothing the strip targets may remain. Uses the exact
    #    deletion predicates (a broad "*pip*" search would false-positive
    #    on the stdlib's pipes.py and the ++pip+pypi_* repo names).
    LEFTOVER=$(find "$TARGET_DIR/parser" \( -type d \( -name "pip" -o -name "pip-*.dist-info" -o -name "ensurepip" \) \) -o \( -type f \( -name "pip" -o -name "pip3*" \) -path "*/bin/*" \))
    if [[ -n "$LEFTOVER" ]]; then
        echo "ERROR: pip/ensurepip artifacts remain in packaged parser:" >&2
        echo "$LEFTOVER" >&2
        exit 1
    fi
    # 2. Positive: the parser's runtime dependencies must still be present.
    #    Checks the importable package directories rather than bazel
    #    repository names, so the check survives repo renames and
    #    rules_python upgrades. (The two deps ship differently today:
    #    antlr4 via a pip repo's site-packages, protobuf via the bazel
    #    protobuf+ module — the path patterns cover both provenances.)
    for PKG in antlr4 google/protobuf; do
        if ! find "$TARGET_DIR/parser" -type d -path "*/$PKG" | grep -q .; then
            echo "ERROR: runtime dependency '$PKG' missing from packaged parser (over-stripped?)" >&2
            exit 1
        fi
    done

    # Include the shell script only for macOS and Linux
    if [[ "$PLATFORM" != windows* ]]; then
        cp "$PROJECT_DIR/fizz" "$TARGET_DIR"
        cp "$SCRIPT_DIR/fizz.env" "$TARGET_DIR"
    fi

    # End-to-end smoke test of the staged release — native platform only
    # (cross-compiled artifacts cannot execute on this host; the linux
    # tarballs additionally get exercised on both architectures by the
    # docker publish workflow downstream).
    if [[ "$PLATFORM" == "$HOST_PLATFORM" ]]; then
        echo "Smoke testing staged release ($PLATFORM is native)..."
        SMOKE_DIR=$(mktemp -d)
        cp "$PROJECT_DIR/examples/references/01-02-atomic-action/Counter.fizz" "$SMOKE_DIR/"
        # The fizzbee binary exits 0 even when the model checker reports
        # FAILED, so the PASSED grep is the real assertion.
        "$TARGET_DIR/fizz" "$SMOKE_DIR/Counter.fizz" | tee "$SMOKE_DIR/smoke.out"
        grep -q "PASSED: Model checker completed successfully" "$SMOKE_DIR/smoke.out"
        rm -rf "$SMOKE_DIR"
    fi

    # Create archives
    if [[ "$PLATFORM" == windows* ]]; then
        zip -r "releases/$TARGET_DIR.zip" "$TARGET_DIR"
    else
        tar -czf "releases/$TARGET_DIR.tar.gz" "$TARGET_DIR"
    fi

    # if cleanup the target dir if DISABLE_CLEANUP is not set
    if [[ "${DISABLE_CLEANUP}" != true ]]; then
      echo "Packaged: $TARGET_DIR"
      rm -rf "$TARGET_DIR"  # Cleanup
    fi
done

echo "All builds completed. Archives are in the releases/ directory."

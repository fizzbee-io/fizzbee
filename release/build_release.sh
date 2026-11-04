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

# Upper bound for the bundled python interpreter executable (and, on
# macOS, libpython). MODULE.bazel points the toolchain at
# python-build-standalone's install_only_stripped archives; the default
# install_only build is the same code plus ~80 MB of DWARF debug data per
# copy, which alone used to triple the linux tarballs. The overrides are
# keyed on the exact python patch version, so a rules_python bump that
# moves "3.13" to a newer release silently reinstates the fat build —
# this check catches that at release time. Stripped sizes as of 3.13.13:
# linux x86_64 30 MB, linux aarch64 21 MB, macOS libpython 17 MB.
MAX_INTERP_BYTES=$((50 * 1024 * 1024))

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
    # venvs_use_declare_symlink=no: since rules_python 2.x py_binary builds a
    # venv next to the runfiles whose bin/python3 is a relative symlink into
    # the toolchain. Copying the tree with -L turns that into a copy of the
    # interpreter, which on macOS then fails to find @rpath/libpython. With
    # the flag off the bootstrap recreates the venv at runtime from the
    # toolchain path instead, and the staged tree contains no symlinks.
    bazel build --platforms=//:"$PLATFORM" \
        --@rules_python//python/config_settings:venvs_use_declare_symlink=no \
        //parser/... //:fizzbee //mbt/generator:generator_bin_zip

    # Create target directory
    TARGET_DIR="$RELEASE_DIR-$PLATFORM"
    # Start from a clean staging dir: a previous aborted run leaves
    # read-only bazel outputs behind that cp cannot overwrite.
    chmod -R u+w "$TARGET_DIR" 2>/dev/null || true
    rm -rf "$TARGET_DIR"
    mkdir -p "$TARGET_DIR"

    # Copy files. The parser is copied entry by entry rather than as the
    # whole bazel-bin/parser directory: since rules_python 2.x that
    # directory also holds test outputs and a top-level _parser_bin.venv
    # whose relative symlinks only resolve inside the runfiles tree (the
    # copy in parser_bin.runfiles/_main/parser/ is the one the bootstrap
    # uses, and -L dereferences it correctly there).
    mkdir -p "$TARGET_DIR/parser"
    for ENTRY in parser_bin parser_bin.repo_mapping parser_bin.runfiles_manifest parser_bin.runfiles; do
        cp -L -R "bazel-bin/parser/$ENTRY" "$TARGET_DIR/parser/"
    done
    # Deduplicate the interpreter: the toolchain ships bin/python and
    # bin/python3 as symlinks to bin/python3.X and `cp -L` just turned them
    # into three full copies (30 MB each on linux x86_64). Restore the
    # symlinks; tar preserves them, and the bootstrap only references
    # bin/python3. The real interpreter is found by name shape so a python
    # minor bump does not need an edit here.
    chmod -R u+w "$TARGET_DIR/parser"
    TOOLCHAIN_DIRS=$(find "$TARGET_DIR/parser/parser_bin.runfiles" -mindepth 1 -maxdepth 1 -type d -name '*python?python_3_*')
    if [[ $(printf '%s\n' "$TOOLCHAIN_DIRS" | grep -c .) -ne 1 ]]; then
        echo "ERROR: expected exactly one bundled python toolchain under parser_bin.runfiles, found:" >&2
        echo "$TOOLCHAIN_DIRS" >&2
        exit 1
    fi
    REAL_PYTHON=$(ls "$TOOLCHAIN_DIRS/bin" | grep -E '^python3\.[0-9]+$')
    if [[ $(printf '%s\n' "$REAL_PYTHON" | grep -c .) -ne 1 ]]; then
        echo "ERROR: expected exactly one bin/python3.X in the bundled toolchain, found: $REAL_PYTHON" >&2
        exit 1
    fi
    for LINK in python python3; do
        if [[ -f "$TOOLCHAIN_DIRS/bin/$LINK" && ! -L "$TOOLCHAIN_DIRS/bin/$LINK" ]]; then
            rm "$TOOLCHAIN_DIRS/bin/$LINK"
            ln -s "$REAL_PYTHON" "$TOOLCHAIN_DIRS/bin/$LINK"
        fi
    done
    # Byte-stream copies (not cp) for the two large single files: on APFS a
    # freshly written bazel output can report a bogus hole (SEEK_HOLE at
    # offset 0 while reads return data), and hole-aware copiers such as
    # macOS cp then write zeros for the first 8 MiB. Observed on macOS 14
    # with a 130 MB generator_bin.zip; cat/dd read the real bytes.
    cat bazel-bin/fizzbee_/fizzbee > "$TARGET_DIR/fizzbee"
    chmod 0755 "$TARGET_DIR/fizzbee"
    cat bazel-bin/mbt/generator/generator_bin.zip > "$TARGET_DIR/mbt_gen.zip"
    # Deduplicate the interpreter inside the zip and verify it. The
    # toolchain ships bin/python and bin/python3 as symlinks to
    # bin/python3.X; bazel's zipper dereferences them, so the zip carried
    # three copies of the interpreter (30 MB each on linux x86_64). The
    # bootstrap (__main__.py) recreates symlink entries on extraction, so
    # the two are rewritten as symlink members pointing at the real file,
    # restoring upstream's layout. The toolchain dir is matched by shape
    # (`*python?python_3_*`, see the invariants below).
    # Then fail loudly if the zip is not intact (the mbt-scaffold
    # subcommand runs it with the user's python3; a corrupt zip would only
    # surface as a user-facing traceback), if an interpreter member is over
    # MAX_INTERP_BYTES (the zip embeds its own toolchain copy), or if the
    # dedupe did not take.
    python3 - "$TARGET_DIR/mbt_gen.zip" "$MAX_INTERP_BYTES" <<'PYEOF'
import fnmatch, os, re, stat, sys, zipfile
path, limit = sys.argv[1], int(sys.argv[2])
tmp = path + ".tmp"
def toolchain_root(name):
    """<...>/<toolchain> if name lies under a python toolchain repo dir, else None."""
    parts = name.split("/")
    for k, part in enumerate(parts[:-1]):
        if fnmatch.fnmatch(part, "*python?python_3_*"):
            return "/".join(parts[:k + 1])
    return None
def toolchain_bin(name):
    """(toolchain dir, basename) if name is <...>/<toolchain>/bin/<file>, else None."""
    parts = name.split("/")
    if len(parts) >= 3 and parts[-2] == "bin" and fnmatch.fnmatch(parts[-3], "*python?python_3_*"):
        return "/".join(parts[:-2]), parts[-1]
    return None
def is_link(i):
    return stat.S_ISLNK(i.external_attr >> 16)
with zipfile.ZipFile(path) as src:
    infos = src.infolist()
    real = sorted({tb[1] for i in infos if (tb := toolchain_bin(i.filename)) and re.fullmatch(r"python3\.\d+", tb[1])})
    if len(real) != 1:
        sys.exit("ERROR: expected exactly one bin/python3.X interpreter in mbt_gen.zip, found %s" % real)
    real = real[0]
    with zipfile.ZipFile(tmp, "w") as dst:
        for i in infos:
            tb = toolchain_bin(i.filename)
            if tb and tb[1] in ("python", "python3") and not is_link(i):
                link = zipfile.ZipInfo(i.filename, date_time=i.date_time)
                link.external_attr = (stat.S_IFLNK | 0o777) << 16
                dst.writestr(link, real)
            else:
                dst.writestr(i, src.read(i))
os.replace(tmp, path)
with zipfile.ZipFile(path) as z:
    bad = z.testzip()
    if bad:
        sys.exit("ERROR: corrupt member in staged mbt_gen.zip: %s" % bad)
    infos = z.infolist()
    # Size invariant: same candidates as the parser check below — the
    # executable plus libpython, which holds the interpreter code on
    # macOS. Zero candidates means the layout changed and the check would
    # be vacuous, so that fails too.
    cands = [i for i in infos if "/bin/python3" in i.filename or "/lib/libpython3" in i.filename]
    if not cands:
        sys.exit("ERROR: no bundled python interpreter found in mbt_gen.zip (layout changed?)")
    fat = [i for i in cands if i.file_size > limit]
    if fat:
        sys.exit("ERROR: bundled interpreter in mbt_gen.zip exceeds %d MB: %s" % (
            limit // 1048576, ", ".join("%s (%d MB)" % (i.filename, i.file_size // 1048576) for i in fat)))
    # Layout invariant, mirroring the parser-tree checks below: exactly one
    # toolchain repo, whose bin/ holds the real interpreter as a regular
    # member and both bin/python and bin/python3 as symlink members
    # pointing at it.
    roots = sorted({r for i in infos if (r := toolchain_root(i.filename))})
    if len(roots) != 1:
        sys.exit("ERROR: expected exactly one python toolchain in mbt_gen.zip, found %s" % roots)
    by_name = {tb[1]: i for i in infos if (tb := toolchain_bin(i.filename))}
    interp = by_name.get(real)
    if interp is None or is_link(interp):
        sys.exit("ERROR: %s/bin/%s in mbt_gen.zip is not a regular interpreter member" % (roots[0], real))
    for name in ("python", "python3"):
        i = by_name.get(name)
        if i is None:
            sys.exit("ERROR: %s/bin/%s missing from mbt_gen.zip" % (roots[0], name))
        if not is_link(i):
            sys.exit("ERROR: %s/bin/%s in mbt_gen.zip is still a copy, not a symlink (interpreter dedupe failed)" % (roots[0], name))
        target = z.read(i).decode()
        if target != real:
            sys.exit("ERROR: %s/bin/%s in mbt_gen.zip links to %r, expected %r" % (roots[0], name, target, real))
PYEOF

    # Strip pip and ensurepip from the bundled hermetic python toolchain.
    # The parser never uses them at runtime, and the stale bundled pip is
    # the only CVE-scanner finding the release tarball itself owns (e.g.
    # CVE-2025-8869 against pip 24.3.1 in the rules_python toolchain;
    # ensurepip embeds another copy of the same pip as a wheel). Removing
    # them clears the findings and shrinks the tarball.
    #
    # Name patterns are deliberately exact: the pip-INSTALLED dependency
    # repos (rules_python++pip+pypi_313_antlr4_python3_runtime_<wheel>_<hash>
    # since rules_python 2.x, formerly ...pypi_312_antlr4_python3_runtime)
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
    # 3. The bundled interpreter is the stripped build (see
    #    MAX_INTERP_BYTES). TOOLCHAIN_DIRS was resolved above (the repo is
    #    matched by shape: `*python?python_3_*` covers rules_python's
    #    `++`/`~~` naming eras). bin/python3* covers the executable;
    #    lib/libpython3* is where the macOS builds keep the interpreter
    #    code (their bin/python3.13 is a stub).
    for F in "$TOOLCHAIN_DIRS"/bin/python3* "$TOOLCHAIN_DIRS"/lib/libpython3*; do
        [[ -f "$F" ]] || continue
        SIZE=$(wc -c < "$F")
        if (( SIZE > MAX_INTERP_BYTES )); then
            echo "ERROR: bundled interpreter $F is $((SIZE / 1048576)) MB, over the $((MAX_INTERP_BYTES / 1048576)) MB limit." >&2
            echo "       Expected python-build-standalone's install_only_stripped flavor. Check that the" >&2
            echo "       python.single_version_platform_override entries in MODULE.bazel still match the" >&2
            echo "       patch version rules_python resolves \"3.13\" to." >&2
            exit 1
        fi
    done
    # 4. The interpreter ships once: bin/python and bin/python3 are symlinks.
    for LINK in python python3; do
        if [[ ! -L "$TOOLCHAIN_DIRS/bin/$LINK" || "$(readlink "$TOOLCHAIN_DIRS/bin/$LINK")" != "$REAL_PYTHON" ]]; then
            echo "ERROR: $TOOLCHAIN_DIRS/bin/$LINK is not a symlink to $REAL_PYTHON (interpreter dedupe failed)" >&2
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

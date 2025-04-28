#!/bin/bash

set -e  # Exit on error

SCRIPT_DIR="$(dirname "$(readlink -f "${BASH_SOURCE[0]:-$0}")")"
PROJECT_DIR="$(dirname $SCRIPT_DIR)"

# Get the current date for versioning if FIZZBEE_RELEASE_VERSION is not set
VERSION="$FIZZBEE_RELEASE_VERSION:-$(date +%Y%m%d)"
RELEASE_DIR="fizzbee-$VERSION"
mkdir -p releases

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
    bazel build --platforms=//:"$PLATFORM" //parser/... //:fizzbee

    # Create target directory
    TARGET_DIR="$RELEASE_DIR-$PLATFORM"
    mkdir -p "$TARGET_DIR"

    # Copy files
    cp -L -R bazel-bin/parser "$TARGET_DIR"
    cp bazel-bin/fizzbee_/fizzbee "$TARGET_DIR"

    # Include the shell script only for macOS and Linux
    if [[ "$PLATFORM" != windows* ]]; then
        cp "$PROJECT_DIR/fizz" "$TARGET_DIR"
    fi

    # Create archives
    if [[ "$PLATFORM" == windows* ]]; then
        zip -r "releases/$TARGET_DIR.zip" "$TARGET_DIR"
    else
        tar -czf "releases/$TARGET_DIR.tar.gz" "$TARGET_DIR"
    fi

    echo "Packaged: $TARGET_DIR"
    rm -rf "$TARGET_DIR"  # Cleanup
done

echo "All builds completed. Archives are in the releases/ directory."

# Thin distribution wrapper around the official release tarball.
#
# The image installs the SAME artifact that brew/tarball users run — the
# self-contained platform release built by release/build_release.sh (fizz
# wrapper, fizzbee binary, parser with its bundled hermetic python
# toolchain, mbt_gen.zip). No bazel, no source build, no repackaging:
# one packaging pipeline, many distribution formats.
#
# Build:  docker build --build-arg FIZZBEE_VERSION=v0.5.3 -t fizzbee .
# Run:    docker run --rm -v "$PWD":/workspace fizzbee path/to/Spec.fizz
#
# FIZZBEE_VERSION is REQUIRED (no default): a baked-in default would go
# stale — months later `docker build .` would silently package an old
# release. Note the image always wraps a PUBLISHED release, never the
# source checkout it is built from.
#
# The fetch stage runs on the BUILD platform (no emulation) and merely
# downloads + extracts the TARGET platform's tarball; the runtime stage
# has no RUN steps. So multi-arch builds (buildx --platform
# linux/amd64,linux/arm64) never execute target-arch code and need no
# QEMU. Note python:slim (not debian:slim): the fizz wrapper needs bash,
# and the mbt-scaffold subcommand launches mbt_gen.zip with the system
# `python`; the parser itself uses the tarball's bundled interpreter.

# ---- Fetch ----------------------------------------------------------------
FROM --platform=$BUILDPLATFORM python:3.12-slim-bookworm AS fetch

ARG FIZZBEE_VERSION
ARG TARGETARCH

# Download with the stage's own python (slim ships no curl/wget) and
# extract. --strip-components drops the fizzbee-<version>-<platform>/
# top-level directory from the tarball.
RUN set -e; \
    if [ -z "$FIZZBEE_VERSION" ]; then \
      echo "ERROR: FIZZBEE_VERSION is required, e.g. --build-arg FIZZBEE_VERSION=v0.5.3" >&2; \
      echo "       (available versions: https://github.com/fizzbee-io/fizzbee/releases)" >&2; \
      exit 1; \
    fi; \
    case "$TARGETARCH" in \
      arm64) PLAT=linux_arm ;; \
      *)     PLAT=linux_x86 ;; \
    esac; \
    URL="https://github.com/fizzbee-io/fizzbee/releases/download/${FIZZBEE_VERSION}/fizzbee-${FIZZBEE_VERSION}-${PLAT}.tar.gz"; \
    echo "Fetching $URL"; \
    python3 -c "import urllib.request,sys; urllib.request.urlretrieve(sys.argv[1], '/tmp/fizzbee.tar.gz')" "$URL"; \
    mkdir -p /opt/fizzbee; \
    tar -xzf /tmp/fizzbee.tar.gz -C /opt/fizzbee --strip-components=1; \
    rm /tmp/fizzbee.tar.gz; \
    test -x /opt/fizzbee/fizz; \
    test -x /opt/fizzbee/fizzbee; \
    test -x /opt/fizzbee/parser/parser_bin

# ---- Runtime ---------------------------------------------------------------
FROM python:3.12-slim-bookworm

COPY --from=fetch /opt/fizzbee /opt/fizzbee

# The fizz wrapper sources fizz.env from its own directory, which points
# PARSER_BIN / FIZZBEE_BIN / MBT_GEN_ZIP at /opt/fizzbee/*.
ENV PATH="/opt/fizzbee:${PATH}"

# Specs are expected to be mounted here: -v "$PWD":/workspace
WORKDIR /workspace

ENTRYPOINT ["/opt/fizzbee/fizz"]

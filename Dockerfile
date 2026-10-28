# Thin distribution wrapper around the official release tarball.
#
# The image installs the SAME artifact that brew/tarball users run — the
# self-contained platform release built by release/build_release.sh (fizz
# wrapper, fizzbee binary, parser, mbt_gen.zip). No bazel, no source
# build: one packaging pipeline, many distribution formats.
#
# Build:  docker build --build-arg FIZZBEE_VERSION=v0.5.3 -t fizzbee .
# Run:    docker run --rm -v "$PWD":/workspace fizzbee path/to/Spec.fizz
#
# FIZZBEE_VERSION is REQUIRED (no default): a baked-in default would go
# stale — months later `docker build .` would silently package an old
# release. Note the image always wraps a PUBLISHED release, never the
# source checkout it is built from.
#
# Python: the tarball bundles a hermetic python toolchain (rules_python /
# python-build-standalone) for the parser and the MBT generator. That
# interpreter statically links its own OpenSSL, expat, sqlite and zlib,
# carries no dpkg/pypi metadata, and so is invisible to CVE scanners and
# receives no security updates between fizzbee releases. The image drops
# it and runs the same parser / generator sources and pip deps on
# Debian's python3 package: dpkg-tracked, CVE-matched by scanners against
# the Debian security tracker, and patched by the monthly rebuild.
#
# Base: debian:trixie-slim + the python3 package, NOT python:3.x-slim.
# The official python image compiles CPython under /usr/local outside
# dpkg; scanners list it but match no CVEs against it (verified with
# docker scout on python:3.12.11-slim-trixie: 0 CPython findings despite
# five missed security releases). Debian's python3.13 is matched exactly.
# debian:trixie-slim already ships bash (Essential) for the fizz wrapper.
#
# The fetch stage runs on the BUILD platform (no emulation): it downloads
# the TARGET platform's tarball and does all the file surgery there. The
# runtime stage has a single RUN (apt-get install python3 plus the
# python-is-python3 alias), which runs under QEMU for the non-native
# half of a multi-arch build.

# ---- Fetch ----------------------------------------------------------------
FROM --platform=$BUILDPLATFORM debian:trixie-slim AS fetch

ARG FIZZBEE_VERSION
ARG TARGETARCH

# The stage's python3 is used to download (slim ships no curl/wget),
# unpack mbt_gen.zip and precompile .pyc. Same Debian python3 as the
# runtime stage so the bytecode magic matches.
RUN apt-get update -qq && \
    apt-get install -y -qq --no-install-recommends python3 ca-certificates && \
    rm -rf /var/lib/apt/lists/*

# Download and extract. --strip-components drops the
# fizzbee-<version>-<platform>/ top-level directory from the tarball.
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

# Replace the bundled python toolchain with launchers for the system python.
#
# 1. parser: delete the toolchain repo from the runfiles tree. The bazel
#    bootstrap (parser_bin) and its manifests hardcode that repo, so they
#    go too; docker/parser-launcher runs parser.main from the remaining
#    runfiles. The toolchain repo is matched by shape (`*python?python_3_*`
#    covers rules_python's `++`/`~~` naming eras) and the deletion is
#    asserted to have removed exactly one directory. A final, name-agnostic
#    sweep fails the build if any interpreter artifact (toolchain dir,
#    executable python3*, libpython*) survives anywhere under /opt/fizzbee.
# 2. mbt generator: generator_bin.zip is a bazel self-extracting zip whose
#    bootstrap re-executes the embedded toolchain, so the zip cannot be
#    used without it. Unpack its runfiles minus the toolchain repo into
#    mbt_gen/; docker/mbt_gen_launcher.py runs mbt.generator.main from
#    there. The fizz wrapper runs `python "$MBT_GEN_ZIP"`, so pointing
#    MBT_GEN_ZIP at the .py launcher needs no wrapper change.
# 3. Precompile to .pyc (bytecode is arch-independent) so the runtime
#    stage never writes __pycache__ into /opt/fizzbee.
# 4. fizz.env from docker/ overrides the tarball's to point the wrapper at
#    the launchers. Invariants at the end fail the build loudly rather
#    than ship a broken image.
COPY docker/fizz.env docker/parser-launcher docker/mbt_gen_launcher.py /opt/fizzbee/
COPY docker/unpack_mbt_gen.py /tmp/
RUN set -e; \
    cd /opt/fizzbee; \
    RF=parser/parser_bin.runfiles; \
    N=$(find "$RF" -mindepth 1 -maxdepth 1 -type d -name '*python?python_3_*' | wc -l); \
    [ "$N" -eq 1 ] || { echo "ERROR: expected exactly 1 bundled python toolchain in $RF, found $N" >&2; ls "$RF" >&2; exit 1; }; \
    find "$RF" -mindepth 1 -maxdepth 1 -type d -name '*python?python_3_*' -exec rm -rf {} +; \
    rm -f parser/parser_bin parser/parser_bin.repo_mapping parser/parser_bin.runfiles_manifest "$RF/MANIFEST"; \
    python3 /tmp/unpack_mbt_gen.py mbt_gen.zip mbt_gen; \
    rm mbt_gen.zip; \
    python3 -m compileall -q parser mbt_gen; \
    chmod 0755 parser-launcher; \
    test -d "$RF/_main/parser"; \
    test -d mbt_gen/runfiles/_main/mbt/generator/templates; \
    ls -d "$RF"/*/site-packages/antlr4 "$RF"/protobuf*/python/google >/dev/null; \
    ls -d mbt_gen/runfiles/*/site-packages/jinja2 mbt_gen/runfiles/*/site-packages/markupsafe >/dev/null; \
    LEFT=$(find /opt/fizzbee \( -type d -name '*python?python_3_*' \) -o \( -type f -name 'python3*' -perm -u+x \) -o \( -type f -name 'libpython*' \)); \
    if [ -n "$LEFT" ]; then echo "ERROR: bundled python interpreter artifacts remain:" >&2; echo "$LEFT" >&2; exit 1; fi; \
    du -sh /opt/fizzbee

# ---- Runtime ---------------------------------------------------------------
FROM debian:trixie-slim

# python3: Debian's interpreter for the launchers (no pip: nothing in the
# image installs packages, and python3-pip is simply never pulled in).
# python-is-python3: the fizz wrapper in current releases invokes
# `python` for mbt-scaffold; Debian ships only `python3` unless this
# package adds the alias. Drop it once the wrapped release calls python3.
RUN apt-get update -qq && \
    apt-get install -y -qq --no-install-recommends python3 python-is-python3 && \
    rm -rf /var/lib/apt/lists/*

COPY --from=fetch /opt/fizzbee /opt/fizzbee

# The fizz wrapper sources fizz.env from its own directory, which points
# PARSER_BIN / FIZZBEE_BIN / MBT_GEN_ZIP at /opt/fizzbee/*.
ENV PATH="/opt/fizzbee:${PATH}"

# Specs are expected to be mounted here: -v "$PWD":/workspace
WORKDIR /workspace

ENTRYPOINT ["/opt/fizzbee/fizz"]

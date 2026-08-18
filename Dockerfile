# Multi-stage build: a heavyweight bazel builder stage that cross-compiles
# for the target architecture, and a slim runtime stage containing only the
# release artifacts. The runtime stage has NO RUN steps, so multi-arch
# builds (buildx --platform linux/amd64,linux/arm64) never execute
# target-arch code — the amd64 builder cross-compiles both.
#
# Build:  docker build -t fizzbee .
# Run:    docker run --rm -v "$PWD":/workspace fizzbee path/to/Spec.fizz
#
# This image intentionally DIVERGES from the release-tarball layout
# (release/build_release.sh) in one way: the parser's bundled hermetic
# rules_python interpreter is replaced with the runtime base image's
# python (see the assembly step below). Tarballs keep the hermetic
# interpreter; the image trades it for one that receives security patches
# with every base-image rebuild. If you change build_release.sh, check
# this file too.

# ---- Builder --------------------------------------------------------------
# Pinned to amd64: gcr.io/bazel-public/bazel publishes no arm64 manifest.
# Version-pinned for reproducible image builds; bump deliberately alongside
# .bazelversion changes.
FROM --platform=linux/amd64 gcr.io/bazel-public/bazel:8.3.1 AS builder

ARG TARGETARCH

WORKDIR /app
COPY . .

RUN case "$TARGETARCH" in \
      arm64) PLATFORM=linux_arm ;; \
      *)     PLATFORM=linux_x86 ;; \
    esac \
 && bazel build --platforms=//:"$PLATFORM" //parser/... //:fizzbee //mbt/generator:generator_bin_zip

# Assemble the runtime layout. Starts from the release-tarball layout
# (fizz, fizz.env, fizzbee, parser/, mbt_gen.zip), then applies the one
# image-specific change:
#
# The parser's runfiles bundle a hermetic rules_python interpreter — a
# 93MB unstripped binary that cp -L materializes THREE times (python,
# python3, python3.12 are symlinks upstream), plus an 89MB libpython:
# ~390MB total, and a frozen interpreter that scanners flag forever.
# Replace the whole toolchain dir with a symlink to the runtime base
# image's interpreter (/usr/local/bin/python3): security patches then
# arrive via `docker pull` after base rebuilds instead of via bazel pin
# bumps. Same minor version (3.12) as the hermetic toolchain.
#
# The stale-venv rm -rf is defensive: rules_python >= 1.9 emits a
# _parser_bin.venv dir that is broken for cross-compiled binaries (its
# interpreter symlink references the host-config toolchain). On the
# current pin (1.0.0) it does not exist; if a future bump reintroduces
# it, it must not ship.
RUN mkdir -p /out \
 && cp -L -R bazel-bin/parser /out/parser \
 && rm -rf /out/parser/_parser_bin.venv \
 && cp -L bazel-bin/fizzbee_/fizzbee /out/fizzbee \
 && cp -L bazel-bin/mbt/generator/generator_bin.zip /out/mbt_gen.zip \
 && cp fizz /out/fizz \
 && cp release/fizz.env /out/fizz.env \
 && chmod -R u+w /out/parser \
 && chmod +x /out/fizz /out/fizzbee /out/parser/parser_bin \
 && PYDIR=$(echo /out/parser/parser_bin.runfiles/rules_python++python+python_3_12_*-unknown-linux-gnu) \
 && rm -rf "$PYDIR" \
 && mkdir -p "$PYDIR/bin" \
 && ln -s /usr/local/bin/python3 "$PYDIR/bin/python3"

# ---- Runtime ---------------------------------------------------------------
# Minor-version pinned: the parser's runfiles point at this image's python,
# so the base must stay on 3.12 (patch releases are exactly the CVE fixes
# we want flowing in; a silent jump to 3.13 could change stdlib behavior).
FROM python:3.12-slim-bookworm

COPY --from=builder /out /opt/fizzbee

# The fizz wrapper sources fizz.env from its own directory, which points
# PARSER_BIN / FIZZBEE_BIN / MBT_GEN_ZIP at /opt/fizzbee/*.
ENV PATH="/opt/fizzbee:${PATH}"

# Specs are expected to be mounted here: -v "$PWD":/workspace
WORKDIR /workspace

ENTRYPOINT ["/opt/fizzbee/fizz"]

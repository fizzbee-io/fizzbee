# Fizzbee

A formal specification language and model checker to specify distributed systems.

Try out now at [Fizzbee Online Playground](https://fizzbee.io/). No installation needed.

# Docs
If you are familiar with [TLA+](https://lamport.azurewebsites.net/tla/tla.html), this would be a quick start:
[From TLA+ to Fizz](https://github.com/fizzbee-io/fizzbee/blob/main/docs/fizzbee-quick-start-for-tlaplus-users.md)

# Installation

On Mac, with brew:
```
brew tap fizzbee-io/fizzbee
brew install fizzbee
```
Alternately, prebuilt binaries are available for macOS and Linux. Please click the link below to get the latest release: [Download the latest release](https://github.com/fizzbee-io/fizzbee/releases/latest).

You can try without installation at https://fizzbee.io/play 


## Online Playground

You can try without installation at https://fizzbee.io/play.

## Pre-built Binary

If you want to run the model checker locally,
Download a correct pre-built release from https://github.com/fizzbee-io/fizzbee/releases,
after extracting downloaded package, run:
```
./fizz path_to_spec.fizz
```

If you are a Mac user,
and have trouble if you download the pre-built binary through browser,
please check https://github.com/fizzbee-io/fizzbee/issues/152.

## Build from Source

Dependencies:

- Bazel: You need bazel installed to build. [Bazelisk](https://github.com/bazelbuild/bazelisk?tab=readme-ov-file#installation) is the recommended way to use bazel. Rename the binary to bazel and put it part of your PATH.
- gcc: This project uses protobuf. Bazel proto_library does not use precompiled protoc, and it builds from scratch. It requires g++ compiler. `sudo apt update; sudo apt install g++`

Build:
```
bazel build parser/parser_bin
bazel build //:fizzbee
```

Run:
```
./fizz path_to_spec.fizz  
```
Example:
```
./fizz examples/tutorials/19-for-stmt-serial-check-again/ForLoop.fizz 
```

Note: Generally, you won't need to rebuild the binary, but most likely will be required after each `git pull`.

### Troubleshooting on macOS

<details>
<summary><strong> 1. Build error on macOS with Protobuf</strong></summary>

If you see a build error in Mac like this:
```
ERROR: /private/var/tmp/_bazel_jp/64463e3d7652188cb285edbcf54b686c/external/protobuf+/src/google/protobuf/io/BUILD.bazel:99:11: Compiling src/google/protobuf/io/printer.cc [for tool] failed: (Exit 1): cc_wrapper.sh failed: error executing CppCompile command (from target @@protobuf+//src/google/protobuf/io:printer) external/rules_cc++cc_configure_extension+local_config_cc/cc_wrapper.sh -U_FORTIFY_SOURCE -fstack-protector -Wall -Wthread-safety -Wself-assign -Wunused-but-set-parameter -Wno-free-nonheap-object ... (remaining 50 arguments skipped)

Use --sandbox_debug to see verbose messages from the sandbox and retain the sandbox build root for debugging
In file included from external/protobuf+/src/google/protobuf/io/printer.cc:12:
bazel-out/darwin_arm64-opt-exec-ST-d57f47055a04/bin/external/protobuf+/src/google/protobuf/io/_virtual_includes/printer/google/protobuf/io/printer.h:918:19: error: 'get<std::function<bool ()>, std::string, std::function<bool ()>>' is unavailable: introduced in macOS 10.13
    value = absl::get<Callback>(that.value);
                  ^
bazel-out/darwin_arm64-opt-exec-ST-d57f47055a04/bin/external/protobuf+/src/google/protobuf/io/_virtual_includes/printer/google/protobuf/io/printer.h:863:11: note: in instantiation of function template specialization 'google::protobuf::io::Printer::ValueImpl<false>::operator=<true>' requested here
    *this = that;
          ^
bazel-out/darwin_arm64-opt-exec-ST-d57f47055a04/bin/external/protobuf+/src/google/protobuf/io/_virtual_includes/printer/google/protobuf/io/printer.h:1150:12: note: in instantiation of function template specialization 'google::protobuf::io::Printer::ValueImpl<false>::ValueImpl<true>' requested here
    return ValueView(it->second);
           ^
/Library/Developer/CommandLineTools/SDKs/MacOSX.sdk/usr/include/c++/v1/variant:1577:22: note: 'get<std::function<bool ()>, std::string, std::function<bool ()>>' has been explicitly marked unavailable here
constexpr const _Tp& get(const variant<_Types...>& __v) {
                     ^
1 error generated.
```
This is a known issue with protobuf compilation in the recent version of protobuf.
You can fix it by adding the following to your `.bazelrc` file:

```
build --host_cxxopt=-std=c++14 --cxxopt=-std=c++14
```
The .bazelrc file is located in the root directory of the project. If it does not exist, you can create it.

That is, run the following command:
```
echo "build --host_cxxopt=-std=c++14 --cxxopt=-std=c++14" >> .bazelrc
```

</details>

<details>
<summary><strong> 2. macOS quarantine warning for prebuilt binaries</strong></summary>

When running the `fizzbee-20250213-macos_arm` binary on macOS Sequoia 15.3 (build 24D60), you may encounter this warning:

```
Apple could not verify "python3" is free of malware that may harm your Mac or compromise your privacy.
```

To fix this, you can download the `macos_arm` release archive manually using this command:

```bash
curl -sL $(curl -s https://api.github.com/repos/fizzbee-io/fizzbee/releases/latest | grep "http.*macos_arm.tar.gz" | awk '{print $2}' | sed 's|[\"\,]*||g') | tar xzvf -
```

</details>

# AI Coding Assistant Skills

FizzBee provides skills for AI coding assistants (Claude Code, Cursor, Gemini CLI, and other tools that support the [Agent Skills](https://agentskills.io) standard). The skills give your AI assistant built-in knowledge of the FizzBee language, how to run the model checker, how to debug specs, and how to write model-based tests — without you having to explain any of it.

Four skills are included:

| Skill | Auto-invoked when... |
|---|---|
| `fizz-spec` | Writing or editing `.fizz` specification files |
| `fizz-check` | Verifying a spec or running the model checker |
| `fizz-debug` | A spec fails, produces unexpected results, or is slow |
| `fizz-mbt` | Writing Go adapter code alongside a `.fizz` spec |

## Install Skills (one-time setup)

If you installed fizzbee via brew, run:

```bash
fizz install-skills
```

This installs skills and reference docs to `~/.claude/skills/`, making them available across all your projects. To preview without making changes:

```bash
fizz install-skills --check
```

To remove:

```bash
fizz install-skills --remove
```

**Without a local fizzbee install**, you can use the standalone script instead:

```bash
curl -fsSL https://raw.githubusercontent.com/fizzbee-io/fizzbee/main/install-claude-skills.sh | bash
```

## Keeping Skills Up to Date

Re-run `fizz install-skills` after upgrading fizzbee to get updated skills and docs:

```bash
brew upgrade fizzbee && fizz install-skills
```

Once installed, just work normally — your AI assistant will automatically use the right skill based on what you're doing. You can also invoke them explicitly: `/fizz-spec`, `/fizz-check`, `/fizz-debug`, `/fizz-mbt`.

# Development

## Bazel build
To run all tests:

```
bazel test //...
```

To regenerate BUILD.bazel files,

```
bazel run //:gazelle
```

To add a new dependency,

```
bazel run //:gazelle -- update-repos github.com/your/repo
```
or
```
gazelle update-repos github.com/your/repo
```

When making grammar changes, run

```
antlr4 -Dlanguage=Python3 -visitor *.g4
```
and commit the py files.
TODO: Automate this using gen-rule, so the generated files are not required in the repository

## Cross compilation to linux
Only the go model checker is cross compiled to linux.

On local machine, run `bazel build //:fizzbee`.

To dockerize or to run on the linux server:
```
bazel build --platforms=//:linux_arm  //:fizzbee
```
or
```
bazel build --platforms=//:linux_x86  //:fizzbee
```

Python seems to work without platforms flag but unfortunately, passing platforms flag actually breaks the build.

# Running Fizz with Docker

Official images are published to GitHub Container Registry for `linux/amd64`
and `linux/arm64`:

```bash
docker pull ghcr.io/fizzbee-io/fizzbee:latest
```

## Run a Spec

Mount the directory containing your spec and pass the spec path relative
to it:

```bash
docker run --rm -v "$PWD":/workspace ghcr.io/fizzbee-io/fizzbee:latest path/to/Spec.fizz
```

Output (state graphs, traces) is written to `out/` next to your spec via
the mount.

## Using a Shell Alias for Easier CLI Access

Add the following to your `.bashrc` or `.zshrc`:

```bash
alias fizz='docker run --rm -v "$PWD":/workspace ghcr.io/fizzbee-io/fizzbee:latest'
```

Then run specs as if fizz were installed locally: `fizz Spec.fizz`.

## Image Tags

- `ghcr.io/fizzbee-io/fizzbee:<version>` (e.g. `0.5.3`) — pinned to a
  FizzBee release. The FizzBee code in a version tag never changes: it
  is always installed from that release's published assets. The image
  digest may still change if the tag is republished on a refreshed
  base image.
- `ghcr.io/fizzbee-io/fizzbee:latest` — the latest release, **rebuilt
  monthly** on a freshly patched base image even when there is no new
  FizzBee release.
- `ghcr.io/fizzbee-io/fizzbee@sha256:...` — an exact immutable image.

Use `latest` in CI to automatically receive OS-level security fixes; a
version tag for a stable FizzBee release; a digest for bit-for-bit
reproducibility.

## Security / CVE Policy

The image is a thin wrapper around the official release tarball — the
exact artifact the pre-built binary installation uses — on a
`python:3.12-slim-trixie` (Debian stable) base. The FizzBee payload is
designed to add **no scanner findings** beyond the base image; verify
the current state yourself:

```bash
docker scout quickview ghcr.io/fizzbee-io/fizzbee:latest
# the Target and Base image rows should show identical counts
```

If a scan attributes findings to the FizzBee payload itself, please
[file an issue](https://github.com/fizzbee-io/fizzbee/issues) so we can
investigate — the policy is to fix or explicitly document any
payload-attributed finding.

Findings inherited from the Debian base are present in every image on
that base. Their status is tracked by the
[Debian Security Tracker](https://security-tracker.debian.org/tracker/)
(see the per-package pages, e.g.
[perl](https://security-tracker.debian.org/tracker/source-package/perl)).
The monthly rebuild re-pulls the latest base image, so Debian fixes are
picked up automatically once they become available in the base image.

## Building the Image Yourself

The Dockerfile packages a published release (it does not build from the
source checkout), so a release version is required:

```bash
docker build --build-arg FIZZBEE_VERSION=v0.5.3 -t fizzbee .
```

To model-check with unreleased code, build from source directly instead
(see [Build from Source](#build-from-source)).

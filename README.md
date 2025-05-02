# fizzbee

A Formal specification language and model checker
to specify distributed systems.
Try out now at [Fizzbee Online Playground](https://fizzbee.io/). No installation needed.

# Docs
If you are familiar with [TLA+](https://lamport.azurewebsites.net/tla/tla.html), this would be a quick start
[From TLA+ to Fizz](https://github.com/fizzbee-io/fizzbee/blob/main/docs/fizzbee-quick-start-for-tlaplus-users.md)

# Run a model checker

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

Note: Generally, you won't need to rebuild the binary,
but most likely will be required after each `git pull`.

### Build error in Mac
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

On local machine, run `bazel build //:fizzbee`

To dockerize or to run on the linux server:
```
bazel build --platforms=//:linux_arm  //:fizzbee
```
or
```
bazel build --platforms=//:linux_x86  //:fizzbee
```
Python seems to work without platforms flag but unfortunately, 
passing platforms flag actually breaks the build.

# Running the Fizz with Docker
This guide will walk you through the steps needed to build and run the application using Docker.

## Clone the Repository 
If you haven't already cloned the project, you can do so by running the following command:

```bash
git clone https://github.com/fizzbee-io/fizzbee.git
cd fizzbee
```

## Build the Docker Image
To build the Docker image, run the following command from the root directory of the project:

```bash
docker build -t fizzbee-app .
```

## Run the Docker Container
Once the image is built, you can run the container using:
```bash
docker run --rm -it fizzbee-app
```

## Using Shell Alias for Easier CLI Access
To make running CLI commands from Docker easier, you can create a shell alias. Add the following to your `.bashrc` or `.zshrc`:

```bash
alias fizz='docker run -it --rm -v $(pwd):/spec -w /spec fizzbee-app'
```

After adding the alias, you will need to either restart your terminal or use the source command to apply the changes immediately:

```bash
source ~/.bashrc # or ~/.zshrc
```

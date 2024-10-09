# fizzbee

A Formal specification language and model checker
to specify distributed systems.
Try out now at [Fizzbee Online Playground](https://fizzbee.io/). No installation needed.

# Docs
If you are familiar with [TLA+](https://lamport.azurewebsites.net/tla/tla.html), this would be a quick start
[From TLA+ to Fizz](https://github.com/fizzbee-io/fizzbee/blob/main/docs/fizzbee-quick-start-for-tlaplus-users.md)

# Run a model checker
For now, no prebuilt binaries. But you can try without installation at https://fizzbee.io/play 

Or Install from source.
- Bazel: You need bazel installed to build. [Bazelisk](https://github.com/bazelbuild/bazelisk?tab=readme-ov-file#installation) is the recommended way to use bazel. Rename the binary to bazel and put it part of your PATH.
- gcc: This project uses protobuf. Bazel proto_library does not use precompiled protoc, and it builds from scratch. It requires g++ compiler. `sudo apt update; sudo apt install g++`


```
./fizz path_to_spec.fizz  
```
Example:
```
./fizz examples/tutorials/19-for-stmt-serial-check-again/ForLoop.fizz 
```

Note: Generally, you won't need to rebuild the binary,
but most likely will be required after each `git pull`.

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
alias fizz='docker run -it --rm -v $(pwd):/app fizzbee-app'
```

After adding the alias, you will need to either restart your terminal or use the source command to apply the changes immediately:

```bash
source ~/.bashrc # or ~/.zshrc
```

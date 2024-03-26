load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive")

http_archive(
    name = "io_bazel_rules_go",
    sha256 = "80a98277ad1311dacd837f9b16db62887702e9f1d1c4c9f796d0121a46c8e184",
    urls = [
        "https://mirror.bazel.build/github.com/bazelbuild/rules_go/releases/download/v0.46.0/rules_go-v0.46.0.zip",
        "https://github.com/bazelbuild/rules_go/releases/download/v0.46.0/rules_go-v0.46.0.zip",
    ],
)

http_archive(
    name = "bazel_gazelle",
    sha256 = "b7387f72efb59f876e4daae42f1d3912d0d45563eac7cb23d1de0b094ab588cf",
    urls = [
        "https://mirror.bazel.build/github.com/bazelbuild/bazel-gazelle/releases/download/v0.34.0/bazel-gazelle-v0.34.0.tar.gz",
        "https://github.com/bazelbuild/bazel-gazelle/releases/download/v0.34.0/bazel-gazelle-v0.34.0.tar.gz",
    ],
)

load("@io_bazel_rules_go//go:deps.bzl", "go_register_toolchains", "go_rules_dependencies")
load("@bazel_gazelle//:deps.bzl", "gazelle_dependencies", "go_repository")
############################################################
# Define your own dependencies here using go_repository.
# Else, dependencies declared by rules_go/gazelle will be used.
# The first declaration of an external repository "wins".
############################################################

# gazelle:repository_macro deps.bzl%go_dependencies
#go_dependencies()

go_repository(
    name = "net_starlark_go",
    importpath = "go.starlark.net",
    sum = "h1:hzy3LFnSN8kuQK8h9tHl4ndF6UruMj47OqwqsS+/Ai4=",
    version = "v0.0.0-20231121155337-90ade8b19d09",
)

go_repository(
    name = "com_github_stretchr_testify",
    importpath = "github.com/stretchr/testify",
    sum = "h1:CcVxjf3Q8PM0mHUKJCdn+eZZtm5yQwehR5yeSVQQcUk=",
    version = "v1.8.4",
)

go_repository(
    name = "com_github_davecgh_go_spew",
    importpath = "github.com/davecgh/go-spew",
    sum = "h1:vj9j/u1bqnvCEfJOwUhtlOARqs3+rkHYY13jYWTU97c=",
    version = "v1.1.1",
)

go_repository(
    name = "in_gopkg_yaml_v3",
    importpath = "gopkg.in/yaml.v3",
    sum = "h1:fxVm/GzAzEWqLHuvctI91KS9hhNmmWOoWu0XTYJS7CA=",
    version = "v3.0.1",
)

go_repository(
    name = "com_github_golang_glog",
    importpath = "github.com/golang/glog",
    sum = "h1:uCdmnmatrKCgMBlM4rMuJZWOkPDqdbZPnrMXDY4gI68=",
    version = "v1.2.0",
)

go_repository(
    name = "com_github_huandu_go_clone",
    importpath = "github.com/huandu/go-clone",
    sum = "h1:3+Aq0Ed8XK+zKkLjE2dfHg0XrpIfcohBE1K+c8Usxoo=",
    version = "v1.7.2",
)

go_repository(
    name = "com_github_zeroflucs_given_generics",
    importpath = "github.com/zeroflucs-given/generics",
    sum = "h1:AU5l2Oil+qNjpGiZWcjpqCILzqz7knDiQjInIzFrOSM=",
    version = "v0.0.0-20231105071439-febf5a852473",
)

go_repository(
    name = "com_github_pkg_profile",
    importpath = "github.com/pkg/profile",
    sum = "h1:hnbDkaNWPCLMO9wGLdBFTIZvzDrDfBM2072E1S9gJkA=",
    version = "v1.7.0",
)

go_repository(
    name = "com_github_felixge_fgprof",
    importpath = "github.com/felixge/fgprof",
    sum = "h1:VvyZxILNuCiUCSXtPtYmmtGvb65nqXh2QFWc0Wpf2/g=",
    version = "v0.9.3",
)

go_repository(
    name = "com_github_google_pprof",
    importpath = "github.com/google/pprof",
    sum = "h1:E/LAvt58di64hlYjx7AsNS6C/ysHWYo+2qPCZKTQhRo=",
    version = "v0.0.0-20240207164012-fb44976bdcd5",
)

go_rules_dependencies()

go_register_toolchains(version = "1.22.0")

gazelle_dependencies()

load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive")

http_archive(
    name = "com_google_protobuf",
    sha256 = "8ff511a64fc46ee792d3fe49a5a1bcad6f7dc50dfbba5a28b0e5b979c17f9871",
    strip_prefix = "protobuf-25.2",
    urls = [
        "https://github.com/protocolbuffers/protobuf/releases/download/v25.2/protobuf-25.2.tar.gz",
        "https://github.com/protocolbuffers/protobuf/archive/v25.2.tar.gz",
    ],
)

load("@com_google_protobuf//:protobuf_deps.bzl", "protobuf_deps")

protobuf_deps()

http_archive(
    name = "rules_python",
    sha256 = "c68bdc4fbec25de5b5493b8819cfc877c4ea299c0dcb15c244c5a00208cde311",
    strip_prefix = "rules_python-0.31.0",
    url = "https://github.com/bazelbuild/rules_python/releases/download/0.31.0/rules_python-0.31.0.tar.gz",
)

load("@rules_python//python:repositories.bzl", "py_repositories")

py_repositories()

#load("@rules_python//python:pip.bzl", "pip_install")
load("@rules_python//python:pip.bzl", "pip_parse")
pip_parse(
   name = "my_deps",
   requirements_lock = "//third_party:requirements_lock.txt",
)
# Load the starlark macro which will define your dependencies.
load("@my_deps//:requirements.bzl", "install_deps")
# Call it to define repos for your requirements.
install_deps()

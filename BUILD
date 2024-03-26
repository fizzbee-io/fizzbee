load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")
load("@bazel_gazelle//:def.bzl", "gazelle")

# gazelle:prefix github.com/jayaprabhakar/fizzbee
gazelle(name = "gazelle")

platform(
    name = "linux_x86",
    constraint_values = [
        "@platforms//os:linux",
        "@platforms//cpu:x86_64",
    ],
)

platform(
    name = "linux_arm",
    constraint_values = [
        "@platforms//os:linux",
        "@platforms//cpu:arm64",
    ],
)

go_library(
    name = "fizzbee_lib",
    srcs = ["main.go"],
    data = ["//examples/ast"],
    importpath = "github.com/jayaprabhakar/fizzbee",
    visibility = ["//visibility:private"],
    deps = [
        "//modelchecker",
        "//proto",
        "@org_golang_google_protobuf//encoding/protojson:go_default_library",
    ],
)

go_binary(
    name = "fizzbee",
    embed = [":fizzbee_lib"],
    visibility = ["//visibility:public"],
)

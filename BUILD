load("@rules_go//go:def.bzl", "go_binary", "go_library")
load("@gazelle//:def.bzl", "gazelle")

# gazelle:prefix github.com/fizzbee-io/fizzbee
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
    importpath = "github.com/fizzbee-io/fizzbee",
    visibility = ["//visibility:private"],
    deps = [
        "//lib",
        "//modelchecker",
        "//proto:proto_go_proto",
        "@org_golang_google_protobuf//encoding/protojson",
        "@org_golang_google_protobuf//proto",
    ],
)

go_binary(
    name = "fizzbee",
    embed = [":fizzbee_lib"],
    visibility = ["//visibility:public"],
)

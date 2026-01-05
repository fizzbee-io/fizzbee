load("@rules_go//go:def.bzl", "go_library")

go_library(
    name = "modelchecker",
    srcs = ["simulation.go"],
    importpath = "github.com/fizzbee-io/fizzbee/pkg/modelchecker",
    visibility = ["//visibility:public"],
    deps = [
        "//modelchecker",
        "//proto:proto_go_proto",
        "@org_golang_google_protobuf//encoding/protojson",
    ],
)

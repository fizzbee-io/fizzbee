module github.com/fizzbee-io/fizzbee

go 1.22.4

require (
	github.com/golang/glog v1.2.0
	github.com/huandu/go-clone v0.0.0
	github.com/stretchr/testify v1.8.4
	go.starlark.net v0.0.0-20231121155337-90ade8b19d09
	golang.org/x/sys v0.17.0
	google.golang.org/protobuf v1.32.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
)

replace github.com/huandu/go-clone => github.com/jayaprabhakar/go-clone v0.0.0-20240501195431-177708839df4

package modelchecker

import (
	"fizz/proto"
	"github.com/fizzbee-io/fizzbee/lib"
)

func ReadOptionsFromYaml(filename string) (*proto.StateSpaceOptions, error) {
	msg, err := ReadOptionsFromYamlRaw(filename)
	if err != nil {
		return nil, err
	}
	if msg.Options.MaxConcurrentActions == 0 {
		msg.Options.MaxConcurrentActions = min(2, msg.Options.MaxActions)
	}
	return msg, err
}

// ReadOptionsFromYamlRaw is ReadOptionsFromYaml without the default for
// max_concurrent_actions, so that the caller can tell an unset limit from
// a set one and pick the default itself.
func ReadOptionsFromYamlRaw(filename string) (*proto.StateSpaceOptions, error) {
	msg := &proto.StateSpaceOptions{}
	err := lib.ReadProtoFromFile(filename, msg)
	if err != nil {
		return nil, err
	}
	if msg.Options == nil {
		msg.Options = &proto.Options{MaxActions: 100}
	}
	return msg, err
}

func ReadOptionsFromYamlString(contents string) (*proto.StateSpaceOptions, error) {
	msg := &proto.StateSpaceOptions{}
	err := lib.ReadProtoFromBytes([]byte(contents), msg)
	if err != nil {
		return nil, err
	}

	return msg, err
}

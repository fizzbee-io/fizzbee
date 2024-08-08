package modelchecker

import (
	"fizz/proto"
	"github.com/fizzbee-io/fizzbee/lib"
)

func ReadOptionsFromYaml(filename string) (*proto.StateSpaceOptions, error) {
	msg := &proto.StateSpaceOptions{}
	err := lib.ReadProtoFromFile(filename, msg)
	if err != nil {
		return nil, err
	}
	if msg.Options == nil {
		msg.Options = &proto.Options{
			MaxActions:            100,
			MaxConcurrentActions:  2,
		}
	}
	if msg.Options.MaxConcurrentActions == 0 {
		msg.Options.MaxConcurrentActions = min(2, msg.Options.MaxActions)
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

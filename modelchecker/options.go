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
		msg.Options.MaxConcurrentActions = msg.Options.MaxActions
	}
	return msg, err
}

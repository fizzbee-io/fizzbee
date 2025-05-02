package modelchecker

import (
	"crypto/sha256"
	"fmt"
	"github.com/fizzbee-io/fizzbee/lib"
	"go.starlark.net/starlark"
)

type ChannelMessage struct {
	receiver string
	frame    *CallFrame
	function string
	params   starlark.StringDict
}

func (cm *ChannelMessage) MarshalJSON() ([]byte, error) {
	return lib.MarshalJSON(map[string]interface{}{
		"receiver": cm.receiver,
		"function": cm.function,
		"params":   cm.params,
	})
}

func (cm *ChannelMessage) Frame() *CallFrame {
	return cm.frame
}

func (cm *ChannelMessage) Receiver() string {
	return cm.receiver
}

func (cm *ChannelMessage) Function() string {
	return cm.function
}

func (cm *ChannelMessage) Params() starlark.StringDict {
	return cm.params
}

func (cm *ChannelMessage) String() string {
	return fmt.Sprintf("ChannelMessage(receiver=%v, function=%s, params=%v)", cm.receiver, cm.function, cm.params)
}

func (cm *ChannelMessage) HashCode() string {
	h := sha256.New()
	h.Write([]byte(cm.String()))
	h.Write([]byte(cm.frame.HashCode()))
	return fmt.Sprintf("%x", h.Sum(nil))
}

func (cm *ChannelMessage) Clone(refs map[starlark.Value]starlark.Value, permutations map[lib.SymmetricValue][]lib.SymmetricValue, alt int) *ChannelMessage {
	frame, err := cm.frame.Clone(refs, permutations, alt)
	PanicOnError(err)
	params := CloneDict(cm.params, refs, permutations, alt)
	PanicOnError(err)
	return &ChannelMessage{
		receiver: cm.receiver,
		frame:    frame,
		function: cm.function,
		params:   params,
	}
}

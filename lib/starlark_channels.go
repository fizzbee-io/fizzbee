package lib

import (
	ast "fizz/proto"
	"fmt"
	"go.starlark.net/starlark"
)

type ChannelMessage struct {
	msg      *ast.Message
	receiver string
	args     starlark.Tuple
	kwargs   []starlark.Tuple
	frame    interface{}
	function string
	params   starlark.StringDict
}

func (cm *ChannelMessage) Frame() interface{} {
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
	if cm.frame == nil {
		return fmt.Sprintf("ChannelMessage(msg=%v, receiver=%v, function=%s, args=%v, kwargs=%v)", cm.msg, cm.receiver, cm.function, cm.args, cm.kwargs)
	} else {
		return fmt.Sprintf("ChannelMessage(msg=%v, receiver=%v, function=%s, params=%v)", cm.msg, cm.receiver, cm.function, cm.params)
	}
}

var (
	nextChannelId = 0
)

func ClearChannelRefs() {
	nextChannelId = 0
}

// Channel represents a custom Starlark object
type Channel struct {
	Id       int    `json:"-"`
	Name     string `json:"name,omitempty"`
	ordering string
	delivery string
	blocking string

	Messages []*ChannelMessage `json:"messages,omitempty"`
}

// Ensure Channel implements starlark.Value
var _ starlark.Value = (*Channel)(nil)
var _ starlark.HasAttrs = (*Channel)(nil)

func (c *Channel) IsSynchronous() bool {
	return c.blocking == "blocking"
}
func (c *Channel) IsOrdered() bool {
	return c.ordering == "ordered"
}

// String returns a string representation of the Channel object
func (c *Channel) String() string {
	return fmt.Sprintf("Channel(Ref=%s, ordering=%q, delivery=%q, blocking=%q, messages=%v)", c.RefStringShort(), c.ordering, c.delivery, c.blocking, c.Messages)
}

// RefStringShort returns a string representation of the Channel object
func (c *Channel) RefStringShort() string {
	if c.Name != "" {
		return c.Name
	} else {
		return fmt.Sprintf("channel#%d", c.Id)
	}
}

// Type returns the type name
func (c *Channel) Type() string {
	return "Channel"
}

// Freeze is required for immutability in Starlark
func (c *Channel) Freeze() {}

// Truth always returns true (non-null)
func (c *Channel) Truth() starlark.Bool {
	return starlark.True
}

// Hash allows Channel to be used as a dictionary key
func (c *Channel) Hash() (uint32, error) {
	return 0, fmt.Errorf("unhashable type: Channel")
}

// Attr gets an attribute (ordering, delivery, blocking)
func (c *Channel) Attr(name string) (starlark.Value, error) {
	switch name {
	case "ordering":
		return starlark.String(c.ordering), nil
	case "delivery":
		return starlark.String(c.delivery), nil
	case "blocking":
		return starlark.String(c.blocking), nil
	case "stub":
		// Return the stub method as a callable Starlark function
		return starlark.NewBuiltin("stub", c.stub), nil
	default:
		return nil, nil
	}
}

// AttrNames returns the list of available attributes
func (c *Channel) AttrNames() []string {
	return []string{"ordering", "delivery", "blocking", "stub"}
}

func CreateChannelBuiltin(channels map[int]*Channel) *starlark.Builtin {
	return starlark.NewBuiltin("Channel", func(t *starlark.Thread, b *starlark.Builtin,
		args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		var ordering, delivery, blocking string

		if err := starlark.UnpackArgs("Channel", args, kwargs,
			"ordering", &ordering,
			"delivery", &delivery,
			"blocking", &blocking,
		); err != nil {
			return nil, err
		}

		newChannel := &Channel{Id: nextChannelId, ordering: ordering, delivery: delivery, blocking: blocking}
		channels[nextChannelId] = newChannel
		nextChannelId++
		return newChannel, nil
	})
}

//
//// NewChannel is the Starlark constructor function
//func NewChannel(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
//	var ordering, delivery, blocking string
//
//	if err := starlark.UnpackArgs("Channel", args, kwargs,
//		"ordering", &ordering,
//		"delivery", &delivery,
//		"blocking", &blocking,
//	); err != nil {
//		return nil, err
//	}
//
//	return &Channel{ordering: ordering, delivery: delivery, blocking: blocking}, nil
//}

func (c *Channel) CloneWithoutMessages() *Channel {
	newChannel := &Channel{Id: c.Id, Name: c.Name, ordering: c.ordering, delivery: c.delivery, blocking: c.blocking}
	//newChannel.Messages = make([]*ChannelMessage, 0, len(c.Messages))
	//for _, message := range c.Messages {
	//	//fmt.Println("cloning message", message)
	//	newChannel.Messages = append(newChannel.Messages, message)
	//}
	return newChannel
}

func (c *Channel) AddMessage(receiver string, frame interface{}, name string, args starlark.StringDict) {
	c.Messages = append(c.Messages, &ChannelMessage{
		receiver: receiver,
		function: name,
		params:   args,
		frame:    frame,
	})
}

// RoleStub wraps an existing Role and associates it with a Channel
type RoleStub struct {
	Role    *Role
	Channel *Channel
}

// Ensure RoleStub implements starlark.Value and HasAttrs
var _ starlark.Value = (*RoleStub)(nil)
var _ starlark.HasAttrs = (*RoleStub)(nil)

// String representation of RoleStub
func (rs *RoleStub) String() string {
	return fmt.Sprintf("RoleStub(role=%s, channel=%s)", (*rs.Role).RefStringShort(), rs.Channel.RefStringShort())
}

func (rs *RoleStub) Type() string         { return "RoleStub" }
func (rs *RoleStub) Freeze()              {}
func (rs *RoleStub) Truth() starlark.Bool { return starlark.True }
func (rs *RoleStub) Hash() (uint32, error) {
	return 0, fmt.Errorf("unhashable type: RoleStub")
}

// Attr allows retrieving attributes and methods of RoleStub
func (rs *RoleStub) Attr(name string) (starlark.Value, error) {
	fmt.Println(name)
	if _, ok := rs.Role.RoleMethods[name]; !ok {
		return rs.Role.Attr(name)
	}
	fmt.Println("name is a role method", name)
	b := starlark.NewBuiltin(name, func(t *starlark.Thread, b *starlark.Builtin,
		args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		fmt.Println("Adding method:", name, "with args", args, "on role", rs.Role)
		rs.Channel.Messages = append(rs.Channel.Messages, &ChannelMessage{msg: nil, receiver: rs.Role.RefStringShort(), function: name, args: args, kwargs: kwargs})
		fmt.Println("channel messages", rs.Channel.Messages)
		return starlark.None, nil
	})

	return b, nil
}

// AttrNames lists available attributes
func (rs *RoleStub) AttrNames() []string {
	return rs.Role.AttrNames()
}

// stub method for Channel, returning a RoleStub
func (c *Channel) stub(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var roleArg starlark.Value

	if err := starlark.UnpackArgs("stub", args, kwargs, "role", &roleArg); err != nil {
		return nil, err
	}
	if role, ok := roleArg.(*Role); ok {
		// Return a RoleStub that references both the Role and the Channel
		return NewRoleStub(role, c), nil
	} else {
		return nil, fmt.Errorf("only roles can be stubbed, cannot stub %s of type %s", roleArg, roleArg.Type())
	}
}

func NewRoleStub(role *Role, c *Channel) *RoleStub {
	return &RoleStub{Role: role, Channel: c}
}

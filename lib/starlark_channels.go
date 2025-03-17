package lib

import (
    "fmt"
    "go.starlark.net/starlark"
)

var (
    nextChannelId = 0
)

func ClearChannelRefs() {
    nextChannelId = 0
}

// Channel represents a custom Starlark object
type Channel struct {
    Id       int    `json:"id"`
    Name     string `json:"name,omitempty"`
    ordering string
    delivery string
    blocking string
}

func (c *Channel) MayDropMessages() bool {
    return c.delivery == "atmost_once"
}

// String returns a string representation of the Channel object
func (c *Channel) String() string {
    return fmt.Sprintf("Channel(Ref=%s, ordering=%q, delivery=%q, blocking=%q)", c.RefStringShort(), c.ordering, c.delivery, c.blocking)
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
        if ordering != "unordered" || delivery != "exactly_once" || blocking != "fire_and_forget" {
            return nil, fmt.Errorf("unsupported channel configuration: ordering=%q, delivery=%q, blocking=%q."+
                " Only (unordered, exactly_once, fire_and_forget) channel is supported at this moment", ordering, delivery, blocking)
        }
        newChannel := &Channel{Id: nextChannelId, ordering: ordering, delivery: delivery, blocking: blocking}
        channels[nextChannelId] = newChannel
        nextChannelId++
        return newChannel, nil
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
    return rs.Role.Attr(name)
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

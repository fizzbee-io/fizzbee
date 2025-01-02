package modelchecker

import (
	ast "fizz/proto"
	"fmt"
	"go.starlark.net/starlark"
	"strings"
)

type RoleDurabilitySpec struct {
	durableFields   map[string]bool
	ephemeralFields map[string]bool
}

type DurabilitySpec struct {
	RoleDurabilitySpec map[string]RoleDurabilitySpec
}

// HasDurabilitySpec returns true if the role has a durability spec.
func (d *DurabilitySpec) HasDurabilitySpec(role string) bool {
	if d.RoleDurabilitySpec == nil {
		return false
	}
	roleDurabilitySpec, ok := d.RoleDurabilitySpec[role]
	if !ok {
		return false
	} else if len(roleDurabilitySpec.durableFields) == 0 && len(roleDurabilitySpec.ephemeralFields) == 0 {
		return false
	}
	return true
}

// IsFieldDurable returns true if the field is durable for the role.
func (d *DurabilitySpec) IsFieldDurable(role string, field string) bool {
	if d.HasDurabilitySpec(role) == false {
		return true
	}

	// If no durabilitySpec for the role, then the field is durable.
	// If the durabilitySpec for the role exists, but both durableFields and ephemeralFields are empty, then the field is durable.
	// If durableFields is not empty, then the field is durable if it is in the durableFields.
	// If ephemeralFields is not empty, then the field is durable if it is not in the ephemeralFields.
	roleDurabilitySpec, _ := d.RoleDurabilitySpec[role]

	if len(roleDurabilitySpec.durableFields) > 0 {
		_, ok := roleDurabilitySpec.durableFields[field]
		return ok
	} else {
		_, ok := roleDurabilitySpec.ephemeralFields[field]
		return !ok
	}
}

func (d *DurabilitySpec) AddDurabilitySpec(evaluator *Evaluator, role *ast.Role) {
	roleName := role.GetName()
	decorators := role.GetDecorators()
	// Find the decorator with name='state'.
	// if the decorator is not found, do nothing
	// if the decorator is found, iterate over the args.
	// if the arg.name == 'durable', get the arg.expr

	stateDecoratorFound := false
	for _, decorator := range decorators {
		if decorator.GetName() != "state" {
			continue
		}
		if stateDecoratorFound {
			panic(NewModelError(decorator.GetSourceInfo(), "Only one state decorator allowed for each role", nil, nil))
		}
		stateDecoratorFound = true
		args := decorator.GetArgs()
		if len(args) == 0 {
			continue
		}
		if len(args) > 1 {
			panic(NewModelError(decorator.GetSourceInfo(), "Exactly one of either 'durable' or 'ephemeral' required", nil, nil))
		}

		for _, arg := range args {
			expr := arg.GetExpr()
			// split expr.PyExpr by '='
			// trim the lhs to get argName, and the rhs is the actual pyExpr
			parts := strings.Split(expr.GetPyExpr(), "=")
			if len(parts) != 2 {
				panic(NewModelError(expr.GetSourceInfo(), fmt.Sprintf("Invalid expression %s for %s decorator in role %s", expr.GetPyExpr(), decorator.GetName(), roleName), nil, nil))
			}
			argName := strings.TrimSpace(parts[0])
			if argName != "durable" && argName != "ephemeral" {
				panic(NewModelError(expr.GetSourceInfo(), fmt.Sprintf("Invalid argument name %s for %s decorator in role %s. Only durable or ephemeral is allowed", argName, decorator.GetName(), roleName), nil, nil))
			}
			pyExpr := strings.TrimSpace(parts[1])

			roleDurabilitySpec, ok := d.RoleDurabilitySpec[roleName]
			if !ok {
				roleDurabilitySpec = RoleDurabilitySpec{durableFields: make(map[string]bool), ephemeralFields: make(map[string]bool)}
			}
			value, err := evaluator.EvalPyExpr(expr.GetSourceInfo().GetFileName(), pyExpr, nil)
			if err != nil {
				panic(NewModelError(expr.GetSourceInfo(), fmt.Sprintf("Error evaluating %s for %s decorator in role %s", pyExpr, decorator.GetName(), roleName), nil, err))
			}
			// value must be starlark.Iterable. Then iterate over the value and add the fields to the durableFields or ephemeralFields
			roleDurabilitySpec.addFields(argName, value)

			d.RoleDurabilitySpec[roleName] = roleDurabilitySpec
		}

	}

}

func (rd *RoleDurabilitySpec) addFields(argName string, value starlark.Value) {
	iterable, ok := value.(starlark.Iterable)
	if !ok {
		panic("value must be iterable")
	}
	iter := iterable.Iterate()
	defer iter.Done()
	var x starlark.Value
	for iter.Next(&x) {
		field := x.(starlark.String).GoString()
		if argName == "durable" {
			rd.durableFields[field] = true
		} else {
			rd.ephemeralFields[field] = true
		}
	}
}

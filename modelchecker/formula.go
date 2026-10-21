package modelchecker

import (
	ast "fizz/proto"
	"fmt"
	"maps"
	"strings"

	"github.com/fizzbee-io/fizzbee/lib"
	"github.com/fizzbee-io/fizzbee/modelchecker/liveness"
	"go.starlark.net/starlark"
	"google.golang.org/protobuf/proto"
)

// toLivenessFormula converts a formula assertion's tree into the engine's
// operators. Atom i of the tree is atom i of the formula.
func toLivenessFormula(f *ast.Formula) *liveness.Formula {
	args := make([]*liveness.Formula, len(f.Args))
	for i, a := range f.Args {
		args[i] = toLivenessFormula(a)
	}
	switch f.Op {
	case ast.Formula_ATOM:
		return liveness.Atom(int(f.Atom))
	case ast.Formula_NOT:
		return liveness.Not(args[0])
	case ast.Formula_AND:
		return liveness.And(args...)
	case ast.Formula_OR:
		return liveness.Or(args...)
	case ast.Formula_IMPLIES:
		return liveness.Implies(args[0], args[1])
	case ast.Formula_ALWAYS:
		return liveness.Always(args[0])
	case ast.Formula_EVENTUALLY:
		return liveness.Eventually(args[0])
	case ast.Formula_UNTIL:
		return liveness.Until(args[0], args[1])
	case ast.Formula_LEADSTO:
		return liveness.LeadsTo(args[0], args[1])
	}
	panic(fmt.Sprintf("unknown formula operator %v in %q", f.Op, f.Text))
}

// classicFormula is the formula a prefixed liveness assertion checks, with
// its witness as atom 0, or nil for other assertions.
func classicFormula(invariant *ast.Invariant) *liveness.Formula {
	alwaysEventually, eventuallyAlways := livenessOperators(invariant)
	switch {
	case alwaysEventually:
		return liveness.Always(liveness.Eventually(liveness.Atom(0)))
	case eventuallyAlways:
		return liveness.Eventually(liveness.Always(liveness.Atom(0)))
	}
	return nil
}

// CheckFormulaAtoms evaluates the atoms of formula assertion index in this
// state. The assertion's block runs once and returns every atom's value.
func CheckFormulaAtoms(process *Process, invariant *ast.Invariant, index int, prober NextStatesProber) uint64 {
	cloned := assertionClone(process, prober)
	result := execFunctionBlock(cloned, invariant.Name, fmt.Sprintf("Invariants[%d]", index))
	list, ok := result.(*starlark.List)
	if !ok {
		panic(fmt.Sprintf("assertion %s: expected the list of atom values, got %s", invariant.Name, result.Type()))
	}
	if list.Len() > 64 {
		panic(fmt.Sprintf("assertion %s has %d atoms; at most 64 are supported", invariant.Name, list.Len()))
	}
	var atoms uint64
	for i := 0; i < list.Len(); i++ {
		if list.Index(i).Truth() {
			atoms |= 1 << uint(i)
		}
	}
	return atoms
}

// ExpandAssertionHeaders replaces every assertion with `for` clauses on its
// header by one instance per binding, named like Base[v1, v2]. Each instance
// binds the clause variables at the top of its body. Iterables and filters
// are evaluated once, over builtins and constants, so the set of instances
// is fixed before Init. Expanding twice is a no-op.
func ExpandAssertionHeaders(file *ast.File, globals starlark.StringDict, evaluator *Evaluator) error {
	expanded := false
	for _, inv := range file.Invariants {
		if len(inv.ForClauses) > 0 {
			expanded = true
		}
	}
	if !expanded {
		return nil
	}
	names := map[string]bool{}
	for _, inv := range file.Invariants {
		if len(inv.ForClauses) == 0 {
			names[inv.Name] = true
		}
	}
	env := maps.Clone(lib.Builtins)
	maps.Copy(env, globals)

	var out []*ast.Invariant
	for _, inv := range file.Invariants {
		if len(inv.ForClauses) == 0 {
			out = append(out, inv)
			continue
		}
		bindings, err := expandForClauses(file.GetSourceInfo().GetFileName(), inv, env, evaluator)
		if err != nil {
			return err
		}
		if len(bindings) == 0 {
			fmt.Printf("WARNING: assertion %s has no instances: its for clauses produce no values\n", inv.Name)
		}
		for _, b := range bindings {
			instance := proto.Clone(inv).(*ast.Invariant)
			instance.ForClauses = nil
			instance.Name = fmt.Sprintf("%s[%s]", inv.Name, strings.Join(b.labels, ", "))
			if names[instance.Name] {
				return fmt.Errorf("assertion %s: instance name %s is used twice; make the values' str() distinct", inv.Name, instance.Name)
			}
			names[instance.Name] = true
			instance.Block.Stmts = append(b.stmts, instance.Block.Stmts...)
			out = append(out, instance)
		}
	}
	file.Invariants = out
	return nil
}

type forBinding struct {
	stmts  []*ast.Statement
	labels []string
}

func expandForClauses(fileName string, inv *ast.Invariant, env starlark.StringDict, evaluator *Evaluator) ([]forBinding, error) {
	var results []forBinding
	var expand func(k int, vars starlark.StringDict, b forBinding) error
	expand = func(k int, vars starlark.StringDict, b forBinding) error {
		if k == len(inv.ForClauses) {
			results = append(results, b)
			return nil
		}
		clause := inv.ForClauses[k]
		fail := func(format string, args ...any) error {
			return fmt.Errorf("%s:%d: assertion %s: %s", fileName, clause.GetSourceInfo().GetStart().GetLine(), inv.Name, fmt.Sprintf(format, args...))
		}
		iterable, err := evaluator.EvalPyExpr(fileName, clause.IterablePyExpr, vars)
		if err != nil {
			return fail("for ... in %s: %v (iterables on an assertion header can use only constants)", clause.IterablePyExpr, err)
		}
		if _, ok := iterable.(*lib.SymmetryDomain); ok {
			return fail("can't iterate over symmetric domain %s on an assertion header yet", clause.IterablePyExpr)
		}
		iter := starlark.Iterate(iterable)
		if iter == nil {
			return fail("%s is a %s, which isn't iterable", clause.IterablePyExpr, iterable.Type())
		}
		defer iter.Done()
		var v starlark.Value
		for i := 0; iter.Next(&v); i++ {
			if _, ok := v.(*lib.SymmetricValue); ok {
				return fail("can't iterate over symmetric values (%s) on an assertion header yet", v)
			}
			values := []starlark.Value{v}
			if len(clause.Targets) > 1 {
				seq, ok := v.(starlark.Indexable)
				if !ok || seq.Len() != len(clause.Targets) {
					return fail("can't unpack %s into %s", v, strings.Join(clause.Targets, ", "))
				}
				values = make([]starlark.Value, seq.Len())
				for j := range values {
					values[j] = seq.Index(j)
				}
			}
			next := maps.Clone(vars)
			for j, t := range clause.Targets {
				next[t] = values[j]
			}
			keep := true
			for _, filter := range clause.FilterPyExprs {
				ok, err := evaluator.EvalPyExpr(fileName, filter, next)
				if err != nil {
					return fail("if %s: %v", filter, err)
				}
				if !ok.Truth() {
					keep = false
					break
				}
			}
			if !keep {
				continue
			}
			nb := forBinding{
				stmts:  append(append([]*ast.Statement{}, b.stmts...), bindingStmt(clause, v, i)),
				labels: append(append([]string{}, b.labels...), labelOf(values)...),
			}
			if err := expand(k+1, next, nb); err != nil {
				return err
			}
		}
		return nil
	}
	if err := expand(0, env, forBinding{}); err != nil {
		return nil, err
	}
	return results, nil
}

// bindingStmt assigns the i-th element v of the clause's iterable to its
// targets. A value with a literal form is written out; anything else is
// looked up by position in the iterable.
func bindingStmt(clause *ast.ForClause, v starlark.Value, i int) *ast.Statement {
	expr := fmt.Sprintf("list(%s)[%d]", clause.IterablePyExpr, i)
	if isLiteral(v) {
		expr = v.String()
	}
	code := fmt.Sprintf("%s = %s", strings.Join(clause.Targets, ", "), expr)
	return &ast.Statement{
		SourceInfo: clause.SourceInfo,
		PyStmt:     &ast.PyStmt{SourceInfo: clause.SourceInfo, Code: code},
	}
}

func isLiteral(v starlark.Value) bool {
	switch v := v.(type) {
	case starlark.String, starlark.Int, starlark.Bool, starlark.NoneType:
		return true
	case starlark.Tuple:
		for _, e := range v {
			if !isLiteral(e) {
				return false
			}
		}
		return true
	}
	return false
}

func labelOf(values []starlark.Value) []string {
	labels := make([]string, len(values))
	for i, v := range values {
		if s, ok := v.(starlark.String); ok {
			labels[i] = string(s)
		} else {
			labels[i] = v.String()
		}
	}
	return labels
}

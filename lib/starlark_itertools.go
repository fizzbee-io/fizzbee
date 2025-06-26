package lib

import (
	"fmt"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

var ItertoolsModule = &starlarkstruct.Module{
	Name: "itertools",
	Members: starlark.StringDict{
		"chain":                         starlark.NewBuiltin("chain", itertoolsChain),
		"combinations":                  starlark.NewBuiltin("combinations", itertoolsCombinations),
		"permutations":                  starlark.NewBuiltin("permutations", itertoolsPermutations),
		"product":                       starlark.NewBuiltin("product", itertoolsProduct),
		"combinations_with_replacement": starlark.NewBuiltin("combinations_with_replacement", itertoolsCombinationsWithReplacement),
	},
}

func itertoolsChain(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var iterables []starlark.Iterable
	if err := starlark.UnpackArgs("chain", args, kwargs, "*", &iterables); err != nil {
		return nil, err
	}
	var result []starlark.Value
	for _, iterable := range iterables {
		iter := iterable.Iterate()
		defer iter.Done()
		var v starlark.Value
		for iter.Next(&v) {
			result = append(result, v)
		}
	}
	return starlark.NewList(result), nil
}

func itertoolsCombinations(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var iterable starlark.Iterable
	var r int
	if err := starlark.UnpackArgs("combinations", args, kwargs, "iterable", &iterable, "r", &r); err != nil {
		return nil, err
	}

	items := []starlark.Value{}
	iter := iterable.Iterate()
	defer iter.Done()
	var v starlark.Value
	for iter.Next(&v) {
		items = append(items, v)
	}
	n := len(items)
	if r > n || r < 0 {
		return starlark.NewList(nil), nil
	}

	var result []starlark.Value
	var comb func(start int, curr []int)
	comb = func(start int, curr []int) {
		if len(curr) == r {
			comb := make([]starlark.Value, r)
			for i, idx := range curr {
				comb[i] = items[idx]
			}
			result = append(result, starlark.Tuple(comb))
			return
		}
		for i := start; i <= n-(r-len(curr)); i++ {
			comb(i+1, append(curr, i))
		}
	}
	comb(0, []int{})
	return starlark.NewList(result), nil
}

func itertoolsPermutations(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var iterable starlark.Iterable
	var r int
	if err := starlark.UnpackArgs("permutations", args, kwargs, "iterable", &iterable, "r", &r); err != nil {
		return nil, err
	}

	items := []starlark.Value{}
	iter := iterable.Iterate()
	defer iter.Done()
	var v starlark.Value
	for iter.Next(&v) {
		items = append(items, v)
	}
	n := len(items)
	if r > n || r < 0 {
		return starlark.NewList(nil), nil
	}

	var result []starlark.Value
	used := make([]bool, n)
	curr := []int{}

	var permute func()
	permute = func() {
		if len(curr) == r {
			p := make([]starlark.Value, r)
			for i, idx := range curr {
				p[i] = items[idx]
			}
			result = append(result, starlark.Tuple(p))
			return
		}
		for i := 0; i < n; i++ {
			if !used[i] {
				used[i] = true
				curr = append(curr, i)
				permute()
				curr = curr[:len(curr)-1]
				used[i] = false
			}
		}
	}
	permute()
	return starlark.NewList(result), nil
}

func itertoolsProduct(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	iterables := make([]starlark.Iterable, len(args))
	for i, arg := range args {
		iter, ok := arg.(starlark.Iterable)
		if !ok {
			return nil, fmt.Errorf("product: argument %d is not iterable", i)
		}
		iterables[i] = iter
	}

	// Materialize all iterables
	pools := make([][]starlark.Value, len(iterables))
	for i, it := range iterables {
		iter := it.Iterate()
		defer iter.Done()
		var v starlark.Value
		for iter.Next(&v) {
			pools[i] = append(pools[i], v)
		}
	}

	var result []starlark.Value
	var product func(int, []starlark.Value)
	product = func(depth int, acc []starlark.Value) {
		if depth == len(pools) {
			result = append(result, starlark.Tuple(acc))
			return
		}
		for _, val := range pools[depth] {
			product(depth+1, append(acc, val))
		}
	}
	product(0, []starlark.Value{})
	return starlark.NewList(result), nil
}

func itertoolsCombinationsWithReplacement(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var iterable starlark.Iterable
	var r int
	if err := starlark.UnpackArgs("combinations_with_replacement", args, kwargs, "iterable", &iterable, "r", &r); err != nil {
		return nil, err
	}

	items := []starlark.Value{}
	iter := iterable.Iterate()
	defer iter.Done()
	var v starlark.Value
	for iter.Next(&v) {
		items = append(items, v)
	}
	n := len(items)
	if r < 0 || n == 0 {
		return starlark.NewList(nil), nil
	}

	var result []starlark.Value
	var comb func(start int, curr []int)
	comb = func(start int, curr []int) {
		if len(curr) == r {
			comb := make([]starlark.Value, r)
			for i, idx := range curr {
				comb[i] = items[idx]
			}
			result = append(result, starlark.Tuple(comb))
			return
		}
		for i := start; i < n; i++ {
			comb(i, append(curr, i)) // allow same index
		}
	}
	comb(0, []int{})
	return starlark.NewList(result), nil
}

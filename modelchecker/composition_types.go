package modelchecker

import "github.com/fizzbee-io/fizzbee/lib"

type HashKey string

type Transition = lib.Pair[*Node, *Node]

// type NodeSet map[*modelchecker.Node]bool
type TransitionSet map[Transition]bool
type JoinHashes map[HashKey][]TransitionSet

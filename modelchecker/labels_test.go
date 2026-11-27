package modelchecker

import (
	"slices"
	"testing"

	ast "fizz/proto"
	"github.com/fizzbee-io/fizzbee/lib"
	"go.starlark.net/starlark"
)

func cappedThread(pcs ...string) *Thread {
	stack := NewCallStack()
	for _, pc := range pcs {
		stack.Push(&CallFrame{pc: pc, scope: &Scope{vars: starlark.StringDict{}}})
	}
	return &Thread{Stack: stack, Copies: lib.NewCardinalityCounter(1)}
}

// threads.inside and t.inside on a MANY handle are exact only because
// capped copies are merged when their whole stacks match, so every copy in
// a group is at the same pcs. This pins that property of mergeCopies: a
// change to its key that merged threads at different pcs would make a MANY
// handle answer for copies that are elsewhere.
func TestMergeCopiesGroupsOnlyIdenticalStacks(t *testing.T) {
	inBlock := "Actions[0].Block.Stmts[1].Block.Stmts[1]"
	outside := "Actions[0].Block.Stmts[2]"
	p := &Process{Threads: []*Thread{
		cappedThread(inBlock),
		cappedThread(inBlock),
		cappedThread(outside),
		cappedThread("Actions[0].Block.Stmts[1].Block.Stmts[0]", "Functions[0].Block.Stmts[0]"),
		cappedThread("Actions[0].Block.Stmts[1].Block.Stmts[0]", "Functions[0].Block.Stmts[1]"),
	}}
	p.mergeCopies()

	var groups []*Thread
	for _, th := range p.Threads {
		if th != nil {
			groups = append(groups, th)
		}
	}
	if len(groups) != 4 {
		t.Fatalf("got %d groups, want 4 (only the two identical stacks merge)", len(groups))
	}
	if !groups[0].Copies.IsMany() || groups[0].Stack.RawArray()[0].pc != inBlock {
		t.Errorf("the two threads at %s should merge into one MANY group", inBlock)
	}
	for _, g := range groups[1:] {
		if !g.Copies.IsOne() {
			t.Errorf("thread at %v merged with a thread at different pcs", g.Stack.RawArray()[0].pc)
		}
	}
}

// Every indexed label path must resolve, through the interpreter's own pc
// resolver, to a Block carrying that label.
func TestLabelPathsResolveLikePcs(t *testing.T) {
	files := []*ast.File{labelsFile()}
	idx := buildLabelIndex(files)
	n := 0
	for key, lt := range idx.targets {
		if lt.label == "" {
			continue
		}
		n++
		b, ok := GetProtoFieldByPath(files[lt.fileIndex], lt.block).(*ast.Block)
		if !ok || b == nil || !slices.Contains(b.GetLabels(), lt.label) {
			t.Errorf("%s: path %s does not resolve to a block labeled %q", key, lt.block, lt.label)
		}
	}
	if n == 0 {
		t.Fatal("no labels indexed")
	}
}

// labelsFile is a small AST: role Server with action Pass (label txn
// around two statements) and func Begin (label w), plus a top-level
// action Global (label g).
func labelsFile() *ast.File {
	py := func(code string) *ast.Statement { return &ast.Statement{PyStmt: &ast.PyStmt{Code: code}} }
	labeled := func(labels ...string) *ast.Statement {
		return &ast.Statement{Block: &ast.Block{Labels: labels, Stmts: []*ast.Statement{py("x = 1"), py("x = 2")}}}
	}
	return &ast.File{
		Roles: []*ast.Role{{
			Name: "Server",
			Actions: []*ast.Action{{Name: "Pass", Block: &ast.Block{Flow: ast.Flow_FLOW_SERIAL,
				Stmts: []*ast.Statement{py("x = 0"), labeled("txn"), py("x = 3")}}}},
			Functions: []*ast.Function{{Name: "Begin", Block: &ast.Block{Flow: ast.Flow_FLOW_SERIAL,
				Stmts: []*ast.Statement{labeled("w")}}}},
		}},
		Actions: []*ast.Action{{Name: "Global", Block: &ast.Block{Flow: ast.Flow_FLOW_SERIAL,
			Stmts: []*ast.Statement{labeled("g")}}}},
	}
}

const (
	passTxnMid = "Roles[0].Actions[0].Block.Stmts[1].Block.Stmts[1]"
	passBefore = "Roles[0].Actions[0].Block.Stmts[1]"
	passAfter  = "Roles[0].Actions[0].Block.Stmts[2]"
	beginWMid  = "Roles[0].Functions[0].Block.Stmts[0].Block.Stmts[1]"
)

func TestLabelIndexResolve(t *testing.T) {
	idx := buildLabelIndex([]*ast.File{labelsFile()})
	if idx.err != nil {
		t.Fatal(idx.err)
	}
	for path, wantPrefix := range map[string]string{
		"Server.Pass.txn": "Roles[0].Actions[0].Block.Stmts[1].Block.",
		"Server.Pass":     "Roles[0].Actions[0].",
		"Server.Begin.w":  "Roles[0].Functions[0].Block.Stmts[0].Block.",
		"Global.g":        "Actions[0].Block.Stmts[0].Block.",
		"Global":          "Actions[0].",
	} {
		lt, err := idx.resolve(path)
		if err != nil {
			t.Errorf("resolve(%q): %v", path, err)
			continue
		}
		if lt.prefix != wantPrefix {
			t.Errorf("resolve(%q).prefix = %q, want %q", path, lt.prefix, wantPrefix)
		}
	}
	for _, bad := range []string{"Server", "Server.Nope", "Server.Pass.nope", "Nope", "Server.Pass.txn.x", "Global.g.x"} {
		if _, err := idx.resolve(bad); err == nil {
			t.Errorf("resolve(%q) succeeded, want an error", bad)
		}
	}
}

func TestLabelIndexErrors(t *testing.T) {
	dup := labelsFile()
	dup.Roles[0].Actions[0].Block.Stmts[2] = &ast.Statement{Block: &ast.Block{Labels: []string{"txn"}}}
	if err := buildLabelIndex([]*ast.File{dup}).err; err == nil {
		t.Error("a label used twice in one action should be an error")
	}
	// A top-level action Server with a label Pass clashes with role
	// Server's action Pass: both are "Server.Pass".
	clash := labelsFile()
	clash.Actions[0].Name = "Server"
	clash.Actions[0].Block.Stmts[0].Block.Labels = []string{"Pass"}
	if err := buildLabelIndex([]*ast.File{clash}).err; err == nil {
		t.Error("a labeled path that also names a role's action should be an error")
	}
	// Without a label, a top-level action named like a role is fine.
	shared := labelsFile()
	shared.Actions[0].Name = "Server"
	shared.Actions[0].Block.Stmts[0].Block.Labels = nil
	idx := buildLabelIndex([]*ast.File{shared})
	if idx.err != nil {
		t.Errorf("a top-level action named like a role, with no clashing label: %v", idx.err)
	}
	for _, path := range []string{"Server", "Server.Pass", "Server.Pass.txn"} {
		if _, err := idx.resolve(path); err != nil {
			t.Errorf("resolve(%q): %v", path, err)
		}
	}
	// Two unlabeled actions with one name are an error only when queried.
	twice := labelsFile()
	twice.Actions = append(twice.Actions, &ast.Action{Name: "Global"})
	idx = buildLabelIndex([]*ast.File{twice})
	if idx.err != nil {
		t.Errorf("an unqueried name clash should not fail the load: %v", idx.err)
	}
	if _, err := idx.resolve("Global"); err == nil {
		t.Error("querying an ambiguous path should be an error")
	}
}

func thread(obj *lib.Role, copies *lib.CardinalityCounter, pcs ...string) *Thread {
	stack := NewCallStack()
	for _, pc := range pcs {
		stack.Push(&CallFrame{pc: pc, obj: obj, scope: &Scope{vars: starlark.StringDict{}}})
	}
	return &Thread{Stack: stack, Copies: copies}
}

func server(id int) *lib.Role {
	return &lib.Role{Name: "Server", ID: starlark.MakeInt(id)}
}

func TestTallyCardinalityAndRoleFilter(t *testing.T) {
	files := []*ast.File{labelsFile()}
	s0, s1 := server(0), server(1)
	many := lib.NewCardinalityCounter(2)
	one := lib.NewCardinalityCounter(1)
	cases := []struct {
		name     string
		threads  []*Thread
		role     *lib.Role
		wantN    int
		wantMany int
		wantCard string
	}{
		{"none", []*Thread{thread(s0, nil, passBefore), thread(s0, nil, passAfter)}, nil, 0, 0, "0"},
		{"one concrete", []*Thread{thread(s0, nil, passTxnMid), thread(s1, nil, passBefore)}, nil, 1, 0, "1"},
		{"two concrete", []*Thread{thread(s0, nil, passTxnMid), thread(s1, nil, passTxnMid)}, nil, 2, 0, "MANY"},
		{"one MANY entry", []*Thread{thread(s0, many, passTxnMid)}, nil, 1, 1, "MANY"},
		{"two ONE entries", []*Thread{thread(s0, one, passTxnMid), thread(s0, one, passTxnMid+".Block.Stmts[0]")}, nil, 2, 0, "MANY"},
		{"concrete plus MANY", []*Thread{thread(s0, nil, passTxnMid), thread(s1, many, passTxnMid)}, nil, 2, 1, "MANY"},
		{"role filter first", []*Thread{thread(s0, nil, passTxnMid), thread(s1, many, passTxnMid)}, s0, 1, 0, "1"},
		{"role filter excludes all", []*Thread{thread(s1, nil, passTxnMid)}, s0, 0, 0, "0"},
	}
	lt, err := buildLabelIndex(files).resolve("Server.Pass.txn")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		p := &Process{Files: files, Threads: c.threads}
		n, m, card := labelTargets{lt}.tally(p, c.role)
		if n != c.wantN || m != c.wantMany || card.String() != c.wantCard {
			t.Errorf("%s: got n=%d many=%d card=%s, want n=%d many=%d card=%s", c.name, n, m, card, c.wantN, c.wantMany, c.wantCard)
		}
	}

	// A list of paths is a union: a thread inside both the action and its
	// txn label is counted once.
	whole, err := buildLabelIndex(files).resolve("Server.Pass")
	if err != nil {
		t.Fatal(err)
	}
	p := &Process{Files: files, Threads: []*Thread{thread(s0, nil, passTxnMid), thread(s1, nil, passBefore)}}
	if n, _, card := (labelTargets{lt, whole}).tally(p, nil); n != 2 || card.String() != "MANY" {
		t.Errorf("union: got n=%d card=%s, want n=2 card=MANY", n, card)
	}
	p = &Process{Files: files, Threads: []*Thread{thread(s0, nil, passTxnMid)}}
	if n, _, card := (labelTargets{lt, whole}).tally(p, nil); n != 1 || card.String() != "1" {
		t.Errorf("union of overlapping paths: got n=%d card=%s, want n=1 card=1", n, card)
	}
}

// One thread whose Server#0 Pass frame is inside txn and which has called
// into another Server's Begin, inside w. The same-thread conjunction with
// two role instances holds for that thread only.
func TestSameThreadAcrossRoleInstances(t *testing.T) {
	files := []*ast.File{labelsFile()}
	s0, s1 := server(0), server(1)
	stack := NewCallStack()
	stack.Push(&CallFrame{pc: passTxnMid, obj: s0, scope: &Scope{vars: starlark.StringDict{}}})
	stack.Push(&CallFrame{pc: beginWMid, obj: s1, scope: &Scope{vars: starlark.StringDict{}}})
	both := &Thread{Stack: stack}
	onlyPass := thread(s0, nil, passTxnMid)
	onlyBegin := thread(s1, nil, beginWMid)

	idx := buildLabelIndex(files)
	txn, _ := idx.resolve("Server.Pass.txn")
	w, _ := idx.resolve("Server.Begin.w")
	check := func(th *Thread, want bool) {
		got := txn.threadMatches(files, th, s0) && w.threadMatches(files, th, s1)
		if got != want {
			t.Errorf("thread %v: same-thread conjunction = %v, want %v", th.Stack.RawArray()[0].pc, got, want)
		}
	}
	check(both, true)
	check(onlyPass, false)
	check(onlyBegin, false)
	// Swapped instances: Server#1 has no Pass frame, Server#0 no Begin frame.
	if txn.threadMatches(files, both, s1) || w.threadMatches(files, both, s0) {
		t.Error("role filter must match the frame's own self, not any frame of the thread")
	}
}

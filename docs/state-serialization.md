# State serialization format

FizzBee can write a complete model-checker state (a `Process`: heap, roles,
threads with their call stacks, channel messages, stats, ...) to a compact
byte string and read it back. Decoding gives a process that the checker can
keep exploring from, the same as one made by `Process.Fork`.

The code is in two layers:

- `lib/statecodec.go`: `StateWriter` / `StateReader` for starlark values,
  and the `CodecContext` shared by a run.
- `modelchecker/statecodec.go`: `EncodeProcess` / `DecodeProcess`, which
  write the process around the values.

It is the base for keeping most of the BFS queue on disk, and later for
sending states between machines. Neither is built yet.

## Goals

1. **Exact.** Aliasing and cycles survive a round trip. If two variables
   point at one list, the decoded state has one list, not two equal ones.
   Naive JSON gets this wrong.
2. **One unit per state.** A whole process is one byte string with one
   reference pool, so a role referenced from the heap, from `Process.Roles`
   and from a frame's `self` decodes to one role.
3. **Compact and cheap.** It is a single pass with no separate pool section
   and no offsets. Ints are varints. Strings the spec already contains are
   written as indexes into a dictionary built from the spec, and the rest
   are written once per state. Record shapes are also written once per
   state.
4. **Extensible.** Value types are one-byte tags, and process fields are
   numbered, so new fields don't need a version bump.

## Layout

```
state   := version:byte  field*  0
field   := fieldNo:uvarint  payload
```

`version` is `lib.StateCodecVersion` (currently 1). A process field with its
zero value is left out. A reader rejects a field number it doesn't know.
So a newer reader still reads older states, and the new field takes its
zero value.

### Primitives

| Name | Encoding |
|---|---|
| uvarint / varint | `encoding/binary` (zigzag for signed) |
| string | uvarint header whose low 2 bits say what it is: `(index<<2)\|2` for a dictionary string; `(len<<2)` + bytes for the first occurrence of any other string; `(index<<2)\|1` for a repeat, with the index into this state's string table |
| raw bytes | uvarint length + bytes (not interned) |
| strings | uvarint count + string* |

### Values

Each value starts with a one-byte tag.

| Tag | Type | Payload | Pooled |
|---|---|---|---|
| 1-3 | None, False, True | none | |
| 4 | int (fits int64) | varint | |
| 5 | int (big) | sign+1 byte, raw big-endian magnitude | |
| 6 | float | 8 bytes LE IEEE 754 | |
| 7 | string | string | via string table |
| 8 | bytes | raw bytes | |
| 9 | tuple | count, values | |
| 10 | **ref** | uvarint index into the value pool | |
| 11 | list | count, values | before elements |
| 12 | dict | count, (key, value)* in insertion order | before entries |
| 13 | set | count, values in insertion order | before elements |
| 14 | record (`lib.Struct`, also role params/fields) | shape, field values | before fields |
| 15 | struct (`starlarkstruct`, enums, composed heaps) | shape, field values | **after** fields |
| 16 | model_value | prefix string, varint id | yes |
| 17 | symmetric_value | prefix, id, kind string, limit | yes |
| 18 | role | name, symmetric bool, id value, has-methods bool, init values (string: their own self-contained encoding), params, fields | after id, before params |
| 19 | RoleStub | role value, channel value | after both |
| 20 | bag | count, values in bag order | before elements |
| 21 | genericmap | count, (key, value)* | before entries |
| 22 | genericset | count, keys | before keys |
| 23 | symmetry_segment | domain value, left, right, flags, resolved id | after domain |
| 24 | Channel | id, name, ordering, delivery, blocking | yes |
| 25 | symmetry_domain | name, limit, kind, divergence, start, flags | yes |
| 26 | symmetric_values | count, values | |
| 27 | range | start, stop, step varints | |
| 28 | opaque | uvarint index into the `CodecContext` (in-process only) | |
| 29 | global | name of a spec global (function or builtin) | |
| 255 | reserved for an extension tag | | |

**The pool.** Every value with identity gets the next pool index when it is
registered. A later occurrence of the same pointer is written as `ref`.
Writer and reader register at the same point, as the table shows:

- Containers register *before* their elements, so a cycle refers back to
  them.
- `starlarkstruct.Struct` is immutable and can't exist before its fields,
  so it registers *after* them. A cycle through it is cut at the mutable
  container inside, the same as `deepCloneStarlarkValue` does.
- A role registers after its id, because its hash depends on the id.

Tuples, ints, strings and other plain values have no identity and aren't
pooled. Strings have their own table, so a repeated string costs 1 to 2
bytes.

**The string dictionary.** Most strings in a state come from the spec:
variable and field names, role and action names, program counters, enum
values. `modelchecker.specDictionary` builds a sorted list of them from the
loaded spec. It contains:

- every identifier in the AST's strings (code, names, literals);
- every statement path, plus its `.any` and `.Block.$` variants;
- `Role.action` and `Role#i.action` names for the first 8 instances of each
  role;
- a few fixed names (`yield`, `init`, `record`, ...).

The list is sorted shortest first, so the common names get one-byte
indexes. It depends only on the spec, so every process running the spec
builds the same list. A string in the dictionary costs 1 to 2 bytes even
the first time it appears in a state. On raft and gossip, about 8 B per
state of strings are still written out in full.

**Shapes.** A record's constructor plus its sorted field names make a
*shape*. The first use writes `0, constructor, names`. Later uses write
`index+1`. Three roles of one type, or a list of records, pay for their
field names once per state.

### Process fields

| No | Field | Payload |
|---|---|---|
| 1 | Name | string |
| 2 | Current | varint |
| 3 | Heap state | string dict: count, (name, value)* in sorted order |
| 4 | Threads | count, then per thread: present bool, id, fairness, aborted, frames |
| 5 | Roles | count, values (refs into the pool) |
| 6 | ChannelMessages | count, (channel id, count, (receiver, function, frame, params)*)* |
| 7 | Channels | count, (id, channel value)* |
| 8 | RotationalLastAllocated | count, (name, varint)* |
| 9 | Stats | total actions, count, (action, varint)* |
| 10 | Returns | string dict |
| 11 | FailedInvariants | count, (key, ints)* |
| 12 | Witness | count, (count, bool*)* |
| 13 | Atoms | count, uvarint* |
| 14 | Labels | strings |
| 15 | Messages | count, proto-encoded `ast.Message`* |
| 16 | Flags | bits: Enabled, ThreadProgress, IsYield, EnableCheckpoint |
| 17 | Fairness | uvarint |
| 18 | ChoiceFairness | uvarint |
| 19 | topLevelVars | strings |

A **frame** is: file index, pc, name, scope chain, vars (string dict),
caller-assign names, and the receiver role (or None). A **scope chain** is
a count, then each scope root-first: flow, skip statements, loop vars, and
loop range (0 means nil, n+1 means n values). The root scope also has its
vars. Child scopes share the root's vars, as they do in memory.

String dicts leave out builtins and modules, as `CopyDict` does when a
state is cloned.

### What is not written

These parts belong to the run, not the state. `DecodeProcess` takes them
from a template process of the same run, the same way `Fork` takes them
from the parent:

- `Files`, `Evaluator`, `SymbolTable`, `Modules`, `Heap.globals`
- `durabilitySpec`, `guidedTrace`, `assertionBindings`

`Parent` is left for the caller to set. Hash caches aren't written and are
recomputed on demand. Transient predicate-evaluation fields aren't written
either.

## CodecContext

A deep clone shares some values between states by pointer instead of copying
them. The context, one per run (`Evaluator.CodecContext()`), keeps that true
after decoding. Everything in it is written **by name or by value**, so a
state decodes in any process running the same spec with the same binary,
not only the one that wrote it:

- **Channels and symmetry domains** are configuration. They are written by
  value and interned by value, so decoding returns the same pointer for
  equal values.
- **Role methods** are the same for every role of a type once its Init has
  run. A role writes only whether it has them yet. The decoder takes them
  from the context, which records them when a role is encoded in this
  process. In another process the context's `RoleResolver` rebuilds them
  from the spec by running the role's top-level statements, as Init does,
  once per role type. A role whose Init hasn't run yet gets its own empty
  map, which Init fills.
- **RoleMethods** (the builtins for a role's fizz functions) are rebuilt
  from the role's function names in the spec, once per role type.
- **Init values** are written by value, as a self-contained encoding that
  goes through the state's string table, so equal init values cost their
  bytes once per state. The decoder interns them by (role type, encoding)
  and hands out one shared struct per distinct value, like Java's
  `readResolve`. Sharing is safe because init values are never changed in
  place: `Role.SetInitValue` replaces the struct. Before this change, Init
  updated the struct in place, and every clone of the role saw the update,
  including states on other branches.
- **Spec globals** (functions, builtins) that end up in a state are written
  by name and looked up in the spec's globals.

The **opaque table** is the fallback for a value with neither an encoding
nor a name, for example a lambda kept in a variable. It stores the value
and writes its index, which only means something inside this process. Specs
in the reference suite don't hit it.

The resolver for a run is set from the first process encoded or decoded
(`modelchecker.specResolver`: the files, globals and evaluator, which are
the same for every process of a run). The context is safe for concurrent
use. A `StateWriter` is not; use one per goroutine (`NewStateWriter` takes
them from a pool).

## Extending

- **New value type:** take the next free tag, add a case to
  `StateWriter.WriteValue` and `StateReader.ReadValue`, and decide where it
  registers in the pool. Add it to `lib/statecodec_test.go`. Older
  encodings still decode.
- **New process field:** add the next field number to
  `modelchecker/statecodec.go`, write it only when non-zero, and read it in
  `readProcessFields`.
- **Changing an existing encoding:** bump `StateCodecVersion`.

## Testing

- `go test ./lib -run StateCodec`: every value type, aliasing across
  top-level values, cycles (including through a struct), roles and stubs,
  and truncated or corrupt input (always an `ErrCorruptState`, never a
  panic or a huge allocation).
- `go test ./modelchecker -run TestStateCodecProcesses`: runs every spec
  in `modelchecker/testdata` and round trips every stored state. The
  decoded state must encode to the same bytes, hash the same and render
  the same JSON.
- The same test also decodes every state with a second run of the spec as
  the template, whose context has seen none of the states. This is what
  another process would do. `testdata/statecodec/RoleMethods.fizz` has role
  methods and an ephemeral field, so this path is covered.
- `FIZZ_STATE_CODEC_ROUNDTRIP=1`: makes `Fork` pass every new process
  through encode and decode, and panic if the result encodes differently.
  `FIZZ_STATE_CODEC_ROUNDTRIP=fresh` does the same, but decodes with a new
  context each time, as another process would. With either set,
  `python3 test_references.py` must still pass all specs with unchanged
  state counts.
- `FIZZ_STATECODEC_SPEC=path/Spec.json go test ./modelchecker -run
  TestStateCodecSpecStats -v` prints size and speed for a spec.

## Measurements

These are from `TestStateCodecSpecStats`, sampling about 2,000 stored
states (linux/amd64). The timings were taken with the run's whole state
graph still live in the heap, so GC adds to all three.

| Spec | Encoded process | Heap state alone | Encode | Decode | Fork (today) |
|---|---:|---:|---:|---:|---:|
| raft (3 nodes) | 225 B | 153 B | 24 us | 9 us | 23 us |
| gossip (3 servers, 1 concurrent) | 176 B | 94 B | 9 us | 7 us | 11 us |

Without the string dictionary, the encoded process was 460 B for raft and
355 B for gossip, and about 60% of that was strings.

For comparison, the memory study measured raft's in-memory `Heap.state` at
2.2 KB per stored node, and the whole node at 5.5 KB. So raft's heap state
is about 14x smaller encoded, and the whole process about 24x smaller than
the node.

## Not done yet, and kept possible

- **Disk spilling of the queue.** States are self-contained byte strings,
  so a batch is just length-prefixed states. The batch header is the place
  for a fingerprint of the string dictionary, so a batch can't be read
  with another spec's dictionary. Compressing a page of states (zstd, for
  example) is a further option on top.
- **Distribution.** States already decode in another process running the
  same spec. The only exception is the opaque fallback above.
- **Hashing.** This encoding isn't canonical: pool order and dict insertion
  order are kept. `lib.StateEncoder` remains the hash input.

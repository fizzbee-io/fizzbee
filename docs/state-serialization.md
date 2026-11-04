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

## Guarantees

| Guarantee | Meaning | Holds |
|---|---|---|
| Round-trip fidelity | The decoded process is the same state: it hashes the same, renders the same, encodes to the same bytes, and keeps every aliasing and cycle, structs included. Unordered collections come back in canonical order. | Always |
| Process portability | The state decodes in another process running the same spec with the same binary. | Unless the state holds an opaque value (a lambda or closure kept across a yield). `CodecContext.SetPortable(true)` makes that an error at encode time, so a portable encoding is never silently process-local. |
| Durable portability | The state decodes after the writing process is gone, for example from a checkpoint on disk. | Not on its own. It needs process portability plus a record of the format version, the spec and dictionary fingerprint and the runtime version. That record belongs in the header of a page of states, which is not built yet; a single state only carries the format version. |

**Cyclic values.** A state may hold cycles (a list holding itself, a
record holding itself, a cycle through several containers), and the
checker explores such states as it does any other: the state hash cuts
values off at depth 20. Decoding gives back the same cycles, and decoded
values behave as the originals do in what the checker does with them: they
hash alike, and comparing a cyclic value with `==` fails alike, with
starlark's "comparison exceeded maximum recursion depth", with or without
the codec. So decoding adds no failure the original state didn't have.
`testdata/statecodec/Cycles.fizz` checks this end to end:
`TestStateCodecForkEquivalence` explores it with Fork as is and with every
fork replaced by its decoded copy, and the state spaces must match.
Limits of the state hash itself (cost on heavily shared values, cyclic
`symmetric_values`) are outside the codec.

Determinism is a separate property from these. Precisely:

- **A fixed object graph always encodes to the same bytes**, aliasing
  included. Heap and scope variables are written in sorted order, ordered
  values (lists, tuples, records, roles, threads) in their own order, and
  unordered collections in canonical order (below), so insertion order
  doesn't matter where the canonical order is total.
- **Canonical order is total only where elements have distinct sort keys.**
  Elements whose keys tie keep their insertion order. Keys tie only for
  elements the state hash can't tell apart either: equal but distinct
  values (two `[[]]` lists in a bag), or cyclic values that differ only
  beyond the hash's depth of 20. Dict and set keys must be hashable, so
  they are immutable and acyclic, and for them ties mean equal keys, which
  a dict or set can't hold twice. So ties come up only in bags and in
  generic collections holding mutable values, and there the bytes can
  depend on insertion order. Decoding is exact either way.
- **The encoding is finer than the hash.** It keeps identity topology: two
  lists sharing a child and two lists with equal children hash the same but
  encode differently, and symmetry reduction happens in the hash only.
  Equal hashes don't imply equal bytes; equal bytes do imply equal hashes.

## Layout

```
state   := version:byte  field*  0
field   := fieldNo:uvarint  payload
```

`version` is `lib.StateCodecVersion` (currently 1). A process field with its
zero value is left out. A reader rejects a field number it doesn't know,
a repeated field and unknown flag bits: compatibility is strict, and only
in one direction. A newer reader still reads older states, and a new
field takes its zero value; an older reader rejects a newer state. Fields
carry no length, so unknown ones can't be skipped; a format meant for
mixed versions would need that, or a version check in a page header.

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
| 12 | dict | count, (key, value)* in canonical order | before entries |
| 13 | set | count, values in canonical order | before elements |
| 14 | record (`lib.Struct`, also role params/fields) | shape, field values | before fields |
| 15 | struct (`starlarkstruct`, enums, composed heaps) | shape, field values | before fields |
| 16 | model_value | prefix string, varint id | yes |
| 17 | symmetric_value | prefix, id, kind string, limit | yes |
| 18 | role | name, symmetric bool, id value, has-methods bool, init values (string: their own self-contained encoding), params, fields | after id, before params |
| 19 | RoleStub | role value, channel value | after both |
| 20 | bag | count, values in canonical order | before elements |
| 21 | genericmap | count, (key, value)* in canonical order | before entries |
| 22 | genericset | count, keys in canonical order | before keys |
| 23 | symmetry_segment | domain value, left, right, flags, resolved id | after domain |
| 24 | Channel | id, name, ordering, delivery, blocking | yes |
| 25 | symmetry_domain | name, limit, kind, divergence, start, flags | yes |
| 26 | symmetric_values | count, values | |
| 27 | range | start, stop, step varints, exactly as the range was made | |
| 28 | opaque | uvarint index into the `CodecContext` (in-process only) | |
| 29 | global | name of a spec global (function or builtin) | |
| 255 | reserved for an extension tag | | |

**The pool.** Every value with identity gets the next pool index when it is
registered. A later occurrence of the same pointer is written as `ref`.
Writer and reader register at the same point, as the table shows:

- Containers, records and structs register *before* their elements, so a
  cycle refers back to them and keeps its identity.
- `starlarkstruct.Struct` has no way to build it before its fields, so the
  reader builds it with its field names and fills the values in as it
  reads them (through a mirror of its unexported layout, checked against
  the real type at startup; a starlark upgrade that changes it makes
  writing a struct fail, and `TestStateCodecStructLayout` catches it).
- A role registers after its id, because its hash depends on the id.

**Canonical order.** Elements of unordered collections (dict, set, bag,
genericmap, genericset) are written sorted, not in insertion order: strings
as strings, int64s as numbers, and any other mix by their `StateEncoder`
bytes. Sort keys are computed per element, apart from the state's pool;
the pool numbering then follows the sorted order, so aliasing between
elements is written consistently. Elements whose keys tie keep their
relative order (see Determinism above). The state hash already ignores
insertion order for these types. A decoded collection iterates in sorted
order.

**Limits.** Values nest at most 10,000 deep, inline (a back-reference is
one level), when writing and reading (`maxValueDepth`). A chain of 10,000
records is the kind of state that would hit it. Measured
(`TestStateCodecDepthAtLimit`), a round trip of lists or records nested
1,000 / 2,000 / 5,000 / 10,000 deep grows the goroutine stack by about
2 / 4 / 8 / 16 MB and takes under 35 ms; Go's default stack limit is 1 GB.

Collection counts and byte lengths are checked against the bytes left
before anything is allocated, so a corrupt length can't request an
arbitrary allocation, and every element and pool entry takes at least one
byte of input. Decoding can still allocate several times the input size,
since runtime containers have their own overhead (a dict's hash table, a
role's maps), and a valid-looking count can allocate before the entries
after it turn out to be corrupt. There are no separate limits on counts,
string lengths or the pool.

The reader never panics: corrupt input, or a value type that panics while
being built, gives an `ErrCorruptState`, and so does a panic while
`DecodeProcess` builds the process around the values. `DecodeProcess` then
returns no process. The reader is strict: bools must be 0 or 1, big ints
must be minimal and outside int64, and a process field may appear only
once, with no unknown flag bits.

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
in the reference suite don't hit it. `CodecContext.SetPortable(true)` turns
the fallback into an error, for encodings that must decode elsewhere. The
table keeps its values alive for the run. Set portability before encoding starts and don't change it during a
run: it applies to every writer sharing the context, so changing it while
states are written concurrently would mix portable and process-local
states. `CodecContext.OpaqueCount()` tells how many values the table
holds (`TestStateCodecSpecStats` prints it).

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
  top-level values, cycles (including through a struct, from either side),
  canonical order (each unordered type built in different orders encodes
  to the same bytes), the depth limit, portable mode, roles and stubs, and
  truncated or corrupt input (always an `ErrCorruptState`, never a panic
  or a huge allocation).
- `go test ./modelchecker -run TestStateCodecForkEquivalence`: the
  statecodec testdata specs (including `Cycles.fizz`) explore the same
  states with and without every fork going through the codec.
- `go test ./modelchecker -run XXX -fuzz FuzzDecodeProcess`: random
  input never makes `DecodeProcess` panic or hang, and a failed decode
  returns no process. Seeds are real states.
- `go test ./lib -run XXX -fuzz FuzzStateReader`: random input never
  panics or hangs the reader, and whatever decodes encodes and decodes
  again. Plain `go test` runs the seeds and the saved crashers in
  `lib/testdata/fuzz`.
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

These are from one run of `TestStateCodecSpecStats` per spec, at the
commit that added this section, sampling about 2,000 stored states
(linux/amd64, a shared cloud container). The timings were taken with the
run's whole state graph still live in the heap, so GC adds to all three,
and they vary by about 30% between runs; sizes don't vary. No spec held an
opaque value.

| Spec | Stored states | Encoded process | Heap state alone | Encode | Decode | Fork (today) |
|---|---:|---:|---:|---:|---:|---:|
| raft (3 nodes) | 328,900 | 225 B | 153 B | 22 us | 14 us | 41 us |
| gossip (3 servers, 1 concurrent) | 2,426 | 176 B | 94 B | 12 us | 9 us | 12 us |
| gossip (3 servers, 2 concurrent) | 86,118 | 215 B | 94 B | 14 us | 9 us | 25 us |

Without the string dictionary, the encoded process was 460 B for raft and
355 B for gossip, and about 60% of that was strings.

For comparison, the memory study measured raft's in-memory `Heap.state` at
2.2 KB per stored node, and the whole node at 5.5 KB. So raft's heap state
is about 14x smaller encoded, and the whole process about 24x smaller than
the node.

### Compressing batches of states

States compress well in batches, because neighbouring states in BFS order
share most of their bytes. Measured on every stored state of raft (328,900)
and gossip with 2 concurrent actions (86,118), in BFS order, one core,
using `github.com/klauspost/compress` (pure Go, not yet a dependency).
Sizes are per state, and so are the times:

| Codec | Alone | 100 per batch | 1,000 per batch | 10,000 per batch | Compress / decompress at 1,000 |
|---|---:|---:|---:|---:|---:|
| none (raft / gossip) | 225 / 215 B | | | | |
| s2 | 192 / 194 B | 30 / 27 B | 27 / 23 B | 26 / 23 B | 0.2 / 0.1 us |
| zstd default | 157 / 164 B | 18 / 17 B | **15 / 14 B** | 15 / 13 B | **0.5 / 0.2 us** |
| zstd better | 157 / 164 B | 16 / 16 B | 13 / 12 B | 12 / 12 B | 0.8 / 0.2 us |
| zstd + dictionary | 100 / 94 B | 18 / 16 B | 15 / 14 B | 15 / 13 B | 0.7 / 0.3 us |

The dictionary was trained on the first 2,000 states and measured on the
rest. It only helps a state compressed on its own; in a batch the earlier
states already act as one. Nothing here compresses yet; this is the
measurement for the queue work built on the codec.

So a batch of 1,000 states with zstd at its default level is about 15x
smaller than the encoded states, for well under 1 us per state, against
20 us to encode and 10 us to decode. Larger batches add little; 100 already
gets most of it. Compressing states one at a time is not worth it (10 us
for a 30% saving).

## Not done yet, and kept possible

- **Disk spilling of the queue.** States are self-contained byte strings,
  so a batch is just length-prefixed states. The batch header is the place
  for a fingerprint of the string dictionary, so a batch can't be read
  with another spec's dictionary. The measurements above suggest
  compressing each batch of about 1,000 states with zstd as one block, in
  memory as well as on disk, with disk pages holding several batches. That
  is an option to evaluate in that work, not a decision made here.
- **Distribution.** States already decode in another process running the
  same spec. The only exception is the opaque fallback above.
- **Hashing.** `lib.StateEncoder` remains the hash input. The encoding is
  a fixed function of the state, but it is finer than the hash: it keeps
  aliasing (one list referenced twice vs two equal lists), which the hash
  doesn't, and symmetry reduction happens in the hash only.

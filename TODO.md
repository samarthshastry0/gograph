# Project Todo

## Phase 3 — Parallel super-steps (Pregel model)

- [ ] Change `Edges` from `map[string]string` to `map[string][]string` and update `NewGraph` to initialize it with `make(map[string][]string)`.
- [ ] Update `AddEdge` to append targets instead of overwriting previous targets.
- [ ] Update `Compile()` to iterate over `for from, tos := range g.Edges` and validate each target in the slice.
- [ ] Handle the `START` edge specially: `len == 0` means no outgoing edges, otherwise validate each target individually.
- [ ] Rewrite `Run` to use a frontier-based super-step loop instead of a single `current` node.
- [ ] Seed the frontier with `Edges[START]` when the entry is `START`; otherwise start with `[]string{entry}`.
- [ ] Add `ctx.Err()` and `maxSteps` checks within the frontier loop to stop on cancellation or step limit.
- [ ] Run each frontier node concurrently with a goroutine per node, `sync.WaitGroup`, and a pre-sized `results` slice keyed by index.
- [ ] Pass `i, name, node` as explicit function parameters to avoid closure capture.
- [ ] Ensure all goroutines read from the same read-only snapshot of state.
- [ ] Capture execution errors into `results[i]` and then wait for all goroutines to finish.
- [ ] Scan results for the first error and return it immediately after `wg.Wait()`.
- [ ] Merge all deltas at the barrier in a single-threaded pass.
- [ ] Apply per-key reducer logic using `channels[key]` and default to overwrite when absent.
- [ ] Keep state mutation isolated: nodes must never mutate the input state; return a fresh delta map each time.
- [ ] Compute the next frontier from all executed nodes using conditional routes or static edges.
- [ ] Skip `END` nodes when building the next frontier.
- [ ] Deduplicate the next frontier with a `map[string]bool` so fan-in only runs each node once per super-step.
- [ ] Add the required `sync` import.
- [ ] Verify the demo graph: `START -> dispatch -> {worker1, worker2, worker3} -> join -> END`.
- [ ] Confirm behavior: `messages` uses append, `done` uses add, and worker execution order is nondeterministic but valid.

## Phase 4 — Serialization & node registry (UI enabler)

- [ ] Add a `reducerRegistry map[string]Reducer` with built-ins such as `"overwrite"`, `"append"`, and `"add"`.
- [ ] Add `NodeFactory func(config map[string]any) (NodeFunc, error)` and a `nodeRegistry` for named node types.
- [ ] Add `RegisterNodeType` to register a factory by type name.
- [ ] Add `routerRegistry` and `RegisterRouter` support.
- [ ] Define JSON-spec structs with proper tags:
  - `GraphSpec{Entry, Nodes, Edges, CondEdges, Channels}`
  - `NodeSpec{ID, Type, Config}`
  - `EdgeSpec`
  - `CondSpec{From, Router, Routes}`
  - `ChannelSpec{Key, Reducer}`
- [ ] Remember the distinction between `ID` (unique graph node name) and `Type` (registered behavior).
- [ ] Implement `BuildGraph(spec) (*Graph, error)`.
- [ ] Resolve all node names and types against the registries.
- [ ] Reuse existing `Add*` helpers while building the graph.
- [ ] Return clear validation errors for unknown names, duplicate IDs, and invalid registrations.
- [ ] Implement `LoadGraph(data []byte)` using `json.Unmarshal` followed by `BuildGraph`.
- [ ] Validate the Phase 3 demo as JSON and confirm `LoadGraph` + `Run` matches the equivalent hand-written graph.
- [ ] Handle JSON number conversions carefully, especially `float64` in factory config values.

## Phase 5 — HTTP/WebSocket API + streaming

- [ ] Add `Stream(ctx, initial) <-chan StepEvent` to emit one `StepEvent` per super-step.
- [ ] Define `StepEvent{Step, Active, State}` and ensure it reflects the active frontier and current state.
- [ ] Add `net/http` endpoints:
  - `POST /graph/validate` to call `Compile()`
  - `POST /graph/run` to call `LoadGraph` + `Run`
- [ ] Add a live streaming endpoint using SSE or WebSocket to emit `StepEvent`s during execution.
- [ ] Configure CORS for the frontend.
- [ ] Validate the endpoint behavior by exercising the Phase 3 demo through the API.

## Phase 6 — Frontend UI (drag-and-drop)

- [ ] Set up a frontend structure under `/graph`, `/registry`, `/server`, and `/web`.
- [ ] Use React + React Flow for the canvas editor.
- [ ] Populate the node palette from the registered node types.
- [ ] Serialize canvas state into the Phase 4 JSON graph spec.
- [ ] Submit serialized graphs to `POST /graph/run`.
- [ ] Highlight the active frontier in the live stream as execution progresses.
- [ ] Verify the browser-based demo matches the JSON-driven graph execution results.

## Phase 7 — Durability & polish

- [ ] Add checkpointing support for Save/Load by thread ID.
- [ ] Add interrupt support before and after a super-step to support pause/resume flows.
- [ ] Expose a UI step button to resume execution one step at a time.
- [ ] Generalize the graph abstraction with generics such as `Graph[S any]`.
- [ ] Add per-node retry logic for transient failures.
- [ ] Support subgraphs and nested graph composition.
- [ ] Add Mermaid or DOT export for graph visualizations.
- [ ] Package the project cleanly for reuse.
- [ ] Add tests around graph compilation, frontier execution, serialization, and HTTP endpoints.
- [ ] Run validation after each phase with `go run .` and fix issues before moving on.

## Execution order

1. Phase 3
2. Phase 4
3. Phase 5
4. Phase 6
5. Phase 7

## Validation cadence

- [ ] After each phase, run `go run .`.
- [ ] Fix compile issues first, then validate behavior and logic.
- [ ] Treat Phase 3 as the critical transition to parallel fan-out/fan-in execution.

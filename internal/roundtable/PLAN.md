# Roundtable implementation

1. [x] Implement bounded room state, routing and cancellation; verify routing, hops,
   independent rooms, history, system prompts and stale replies with package tests.
2. [x] Implement the CLI adapter and bounded prompt/output handling; verify real helper
   processes receive cwd/env/stdin/argv and terminate on cancellation or timeout.
3. [x] Run the package tests with the race detector and cross-compile Windows tests.

Plan check: adapters must run outside the room lock, callbacks need a generation
check, and process cancellation must kill descendants holding output pipes. Keep
ACP outside this package; adapters receive the same Message contract.

Verification (2026-11-15): package tests passed (2.472s), final race tests passed
(3.665s), and Windows amd64 test compilation passed. Real subprocess tests cover
stdin/argv, cwd/env, stderr retention, UTF-8/output limits, timeout/cancellation,
and Unix descendant cleanup after both a running and an exited wrapper.
Routing tests also prove only leading mentions select an agent, unknown or
disabled targets never fall through, and invalid sends preserve active calls.

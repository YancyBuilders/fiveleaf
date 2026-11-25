# Editor ACP client

This package launches an explicitly configured executable and speaks ACP v1 over
newline-delimited JSON-RPC. The editor calls `Start`, `Initialize`, `NewSession`
and `Prompt` from a background goroutine. `Initialize` exposes authentication
methods and prompt capabilities; `Authenticate` accepts advertised agent-managed
login methods. Model configuration and credentials stay with the agent.

`Handler.Update` receives notifications in order and must return promptly. The
other callbacks may wait for the UI, but must stop when their context is cancelled.
File reads return the complete current editor buffer; this package applies ACP's
one-based line range. Writes must preserve unsaved user edits: the editor decides
whether to apply, ask, or report a conflict. A handler that waits for the UI should
call `ResolvePath` again immediately before file access.

Use `Cancel` to stop a turn and wait for its `Prompt` response before sending
another. Cancelling the prompt context also cancels pending permission callbacks,
but a session whose turn has not acknowledged completion cannot accept a further
prompt. `Close` terminates the process, cancels callbacks and waits for cleanup.
On Unix the adapter and its child process group are terminated together.

The client advertises text file read/write support. It does **not** advertise
terminal execution or interactive terminal authentication; all `terminal/*`
requests are rejected. ACP adapters may execute their own tools, so this is not a
sandbox. This package checks each file request against the session and resolved
workspace paths, rejects `.git` metadata, and bounds protocol messages, text and
stderr. The host editor owns permission UI and protection of editor/agent config
files. stderr is retained only in a bounded buffer and is not logged automatically.

The protocol transport and wire types come from `github.com/coder/acp-go-sdk`
v0.13.5. The package uses its lower-level `Connection`: this lets a valid
`fs/write_text_file` request contain `content: ""`, which that version's generated
client validator incorrectly rejects. Missing and null content are still errors.

References checked during implementation:

- <https://agentclientprotocol.com/protocol/v1/initialization>
- <https://agentclientprotocol.com/protocol/v1/prompt-turn>
- <https://agentclientprotocol.com/protocol/v1/tool-calls>
- <https://agentclientprotocol.com/protocol/v1/file-system>
- <https://agentclientprotocol.com/protocol/v1/transports>
- <https://zed.dev/docs/ai/external-agents>
- <https://zed.dev/docs/ai/agent-panel>

Run `go test -race ./internal/acp` for the real-stdio fake-agent integration tests.

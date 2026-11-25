// Package lsptest is a fake language server, for the tests of the LSP
// client and of the app: a test binary runs as one when asked.
package lsptest

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Serve answers as a language server of a language whose every word
// "bad" is an error, on in and out, until it is told to exit.
func Serve(in io.Reader, out io.Writer) {
	r := bufio.NewReader(in)
	send := func(m any) {
		data, _ := json.Marshal(m)
		fmt.Fprintf(out, "Content-Length: %d\r\n\r\n%s", len(data), data)
	}
	var initialize, document, definition, hover, completion json.RawMessage
	var definitionDocument, hoverDocument json.RawMessage
	documents := map[string]json.RawMessage{}
	var definitionCount, hoverCount int
	var heldDefinitions, heldHovers []int
	var heldCompletion int
	var options struct {
		HoverContents       json.RawMessage `json:"hoverContents"`
		DefinitionURI       string          `json:"definitionUri"`
		DefinitionLine      int             `json:"definitionLine"`
		DefinitionCharacter int             `json:"definitionCharacter"`
		HoldDefinitions     bool            `json:"holdDefinitions"`
		HoldHovers          bool            `json:"holdHovers"`
		HoldCompletions     bool            `json:"holdCompletions"`
	}
	definitionReply := func(id int) {
		uri, line, col := options.DefinitionURI, options.DefinitionLine, options.DefinitionCharacter
		if uri == "" {
			uri, line, col = "file:///tmp/def.fake", 3, 1
		}
		send(map[string]any{"jsonrpc": "2.0", "id": id, "result": []any{map[string]any{"uri": uri, "range": map[string]any{
			"start": map[string]any{"line": line, "character": col}, "end": map[string]any{"line": line, "character": col + 3}}}}})
	}
	for {
		n := -1
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			line = strings.TrimSpace(line)
			if line == "" {
				break
			}
			if v, ok := strings.CutPrefix(line, "Content-Length:"); ok {
				n, _ = strconv.Atoi(strings.TrimSpace(v))
			}
		}
		body := make([]byte, n)
		io.ReadFull(r, body)
		var m struct {
			ID     *int            `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		json.Unmarshal(body, &m)
		reply := func(result any) { send(map[string]any{"jsonrpc": "2.0", "id": *m.ID, "result": result}) }
		switch m.Method {
		case "initialize":
			initialize = append(json.RawMessage(nil), m.Params...)
			var p struct {
				InitializationOptions json.RawMessage `json:"initializationOptions"`
			}
			json.Unmarshal(m.Params, &p)
			json.Unmarshal(p.InitializationOptions, &options)
			reply(map[string]any{"capabilities": map[string]any{
				"hoverProvider": true, "definitionProvider": true, "completionProvider": map[string]any{"triggerCharacters": []string{"."}},
				"documentFormattingProvider": true, "textDocumentSync": 1,
			}})
		case "textDocument/didOpen", "textDocument/didChange":
			document = append(json.RawMessage(nil), m.Params...)
			var p struct {
				TextDocument struct {
					URI  string `json:"uri"`
					Text string `json:"text"`
				} `json:"textDocument"`
				ContentChanges []struct {
					Text string `json:"text"`
				} `json:"contentChanges"`
			}
			json.Unmarshal(m.Params, &p)
			documents[p.TextDocument.URI] = document
			text := p.TextDocument.Text
			if len(p.ContentChanges) > 0 {
				text = p.ContentChanges[0].Text
			}
			var diags []any
			for i, line := range strings.Split(text, "\n") {
				if at := strings.Index(line, "bad"); at >= 0 {
					diags = append(diags, map[string]any{"range": map[string]any{
						"start": map[string]any{"line": i, "character": at}, "end": map[string]any{"line": i, "character": at + 3},
					}, "severity": 1, "message": "bad word", "source": "fake"})
				}
			}
			send(map[string]any{"jsonrpc": "2.0", "method": "textDocument/publishDiagnostics",
				"params": map[string]any{"uri": p.TextDocument.URI, "diagnostics": diags}})
		case "textDocument/hover":
			hover, hoverCount = append(json.RawMessage(nil), m.Params...), hoverCount+1
			var p struct{ TextDocument struct{ URI string } }
			json.Unmarshal(m.Params, &p)
			hoverDocument = documents[p.TextDocument.URI]
			if options.HoldHovers {
				heldHovers = append(heldHovers, *m.ID)
			} else if len(options.HoverContents) > 0 {
				reply(map[string]any{"contents": options.HoverContents})
			} else {
				reply(map[string]any{"contents": map[string]any{"kind": "markdown", "value": "**hello**"}})
			}
		case "test/snapshot":
			reply(map[string]any{"initialize": initialize, "document": document, "completion": completion,
				"definition": definition, "definitionCount": definitionCount, "definitionDocument": definitionDocument,
				"hover": hover, "hoverCount": hoverCount, "hoverDocument": hoverDocument})
		case "test/releaseDefinitions":
			for _, id := range heldDefinitions {
				if id != 0 {
					definitionReply(id)
				}
			}
			heldDefinitions = nil
			reply(nil)
		case "test/replyDefinition", "test/replyHover":
			var p struct {
				Index, Line, Character int
				URI, Text              string
			}
			json.Unmarshal(m.Params, &p)
			held := heldDefinitions
			if m.Method == "test/replyHover" {
				held = heldHovers
			}
			if p.Index < 0 || p.Index >= len(held) || held[p.Index] == 0 {
				reply(false)
				continue
			}
			var result any
			if m.Method == "test/replyHover" {
				result = map[string]any{"contents": map[string]any{"kind": "markdown", "value": p.Text}}
			} else {
				pos := map[string]any{"line": p.Line, "character": p.Character}
				result = []any{map[string]any{"uri": p.URI, "range": map[string]any{"start": pos, "end": pos}}}
			}
			send(map[string]any{"jsonrpc": "2.0", "id": held[p.Index], "result": result})
			held[p.Index] = 0
			reply(true)
		case "test/noReply":
			// A response timeout must not disconnect an otherwise healthy client.
		case "textDocument/definition":
			definition, definitionCount = append(json.RawMessage(nil), m.Params...), definitionCount+1
			var p struct{ TextDocument struct{ URI string } }
			json.Unmarshal(m.Params, &p)
			definitionDocument = documents[p.TextDocument.URI]
			if options.HoldDefinitions {
				heldDefinitions = append(heldDefinitions, *m.ID)
			} else {
				definitionReply(*m.ID)
			}
		case "textDocument/completion":
			completion = append(json.RawMessage(nil), m.Params...)
			if options.HoldCompletions {
				heldCompletion = *m.ID
			} else {
				reply(map[string]any{"isIncomplete": false, "items": []any{map[string]any{"label": "println", "insertText": "println()"}}})
			}
		case "test/replyCompletion":
			if heldCompletion != 0 {
				send(map[string]any{"jsonrpc": "2.0", "id": heldCompletion, "result": []any{map[string]any{"label": "stale"}}})
			}
			reply(heldCompletion != 0)
			heldCompletion = 0
		case "textDocument/formatting":
			reply([]any{map[string]any{"range": map[string]any{
				"start": map[string]any{"line": 0, "character": 0}, "end": map[string]any{"line": 0, "character": 0}}, "newText": "// formatted\n"}})
		case "shutdown":
			reply(nil)
		case "exit":
			return
		default:
			if m.ID != nil {
				reply(nil)
			}
		}
	}
}

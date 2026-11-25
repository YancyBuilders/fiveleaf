package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRoleClientFileStdinAndSocket(t *testing.T) {
	dir := t.TempDir()
	socket := SocketPath(dir)
	listener, err := Listen(socket)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	requests := make(chan Request, 3)
	go Serve(listener, func(req Request, reply func(Reply)) {
		requests <- req
		reply(Reply{Data: json.RawMessage(`{"created":2,"presetsApplied":0}`)})
	})
	bundle := `{"version":1,"roles":{"审稿":{"agent":"local"},"助理":{"followCurrentAgent":true}}}`
	if err := os.WriteFile(filepath.Join(dir, "roles.json"), []byte(bundle), 0o600); err != nil {
		t.Fatal(err)
	}
	client := Client{Socket: socket}
	run := func(args ...string) (int, string, string) {
		var out, errs bytes.Buffer
		status, _ := client.Run(append([]string{"roles"}, args...), dir, &out, &errs)
		return status, out.String(), errs.String()
	}
	if status, output, _ := run("socket"); status != 0 || strings.TrimSpace(output) != socket {
		t.Fatal("socket discovery did not return the active endpoint")
	}
	for _, source := range []string{"roles.json", "-"} {
		client.Stdin = strings.NewReader(bundle)
		if status, output, message := run("import", source); status != 0 || !json.Valid([]byte(output)) {
			t.Fatalf("import %q: %d %s", source, status, message)
		}
		if req := <-requests; req.Roles == nil || req.Roles.Action != "import" || string(req.Roles.Bundle) != bundle || req.Plain() {
			t.Fatalf("wrong wire request: %+v", req.Roles)
		}
	}
	client.Stdin = strings.NewReader(`{"version":1} {}`)
	if status, _, message := run("import"); status == 0 || message == "" || len(requests) != 0 {
		t.Fatal("invalid stdin reached the socket or succeeded")
	}
}

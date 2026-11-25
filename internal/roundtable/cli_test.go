package roundtable

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

const helperModeEnv = "FIVELEAF_ROUNDTABLE_TEST_HELPER"

type cliInput struct {
	Dir   string
	Value string
	Args  []string
	Stdin string
}

// Run the actual test executable as an agent, so exec, pipes, cwd, environment,
// argument handling and process cancellation are exercised together.
func TestRoundtableCLIProcess(t *testing.T) {
	mode := os.Getenv(helperModeEnv)
	if mode == "" {
		return
	}
	args := os.Args
	for len(args) > 0 && args[0] != "--" {
		args = args[1:]
	}
	if len(args) > 0 {
		args = args[1:]
	}
	switch mode {
	case "echo":
		input, err := io.ReadAll(os.Stdin)
		if err != nil {
			os.Exit(2)
		}
		dir, _ := os.Getwd()
		_ = json.NewEncoder(os.Stdout).Encode(cliInput{dir, os.Getenv("FIVELEAF_ROUNDTABLE_TEST_VALUE"), args, string(input)})
	case "failure":
		_, _ = io.WriteString(os.Stdout, "partial answer")
		_, _ = io.WriteString(os.Stderr, strings.Repeat("界", MaxStderrBytes)+"\nlast diagnostic\n")
		os.Exit(23)
	case "invalid-utf8":
		_, _ = os.Stdout.Write([]byte{'o', 'k', 0xff, '\n'})
	case "overflow":
		_, _ = io.WriteString(os.Stdout, strings.Repeat("x", MaxOutputBytes+4096))
		time.Sleep(30 * time.Second)
	case "wait":
		if path := os.Getenv("FIVELEAF_ROUNDTABLE_TEST_READY"); path != "" {
			_ = os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())), 0600)
		}
		time.Sleep(30 * time.Second)
	case "descendant-parent", "descendant-exit":
		child := exec.Command(os.Args[0], "-test.run=^TestRoundtableCLIProcess$")
		child.Env = append(os.Environ(), helperModeEnv+"=wait")
		child.Stdout, child.Stderr = os.Stdout, os.Stderr
		if err := child.Start(); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		if mode == "descendant-parent" {
			_ = child.Wait()
		}
	default:
		os.Exit(2)
	}
	os.Exit(0)
}

func helperCLI(mode string) CLIAdapter {
	return CLIAdapter{
		Command: os.Args[0],
		Args:    []string{"-test.run=^TestRoundtableCLIProcess$", "--"},
		Env: map[string]string{
			helperModeEnv: mode,
			"GORACE":      "atexit_sleep_ms=0",
		},
		Timeout: 10 * time.Second,
	}
}

func waitForFile(t *testing.T, path string) []byte {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(path); err == nil && len(data) > 0 {
			return data
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("helper did not create %s", path)
	return nil
}

func TestCLIAdapterPassesWorkingDirectoryEnvironmentAndPrompt(t *testing.T) {
	messages := []Message{{Role: "system", Text: "review carefully"}, {Role: "user", Text: "a prompt with 'quotes', $HOME and 中文"}}
	for _, argument := range []bool{false, true} {
		t.Run(fmt.Sprintf("argument=%v", argument), func(t *testing.T) {
			dir := t.TempDir()
			literal := "$(touch " + filepath.Join(dir, "must-not-exist") + "); $HOME 'literal'"
			adapter := helperCLI("echo")
			adapter.Dir = dir
			adapter.Args = append(adapter.Args, literal)
			adapter.Env["FIVELEAF_ROUNDTABLE_TEST_VALUE"] = "override with 空格"
			adapter.PromptArgument = argument
			message, err := adapter.Call(context.Background(), messages)
			if err != nil {
				t.Fatal(err)
			}
			var got cliInput
			if err := json.Unmarshal([]byte(message.Text), &got); err != nil {
				t.Fatalf("decode %q: %v", message.Text, err)
			}
			wantDir, _ := filepath.EvalSymlinks(dir)
			gotDir, _ := filepath.EvalSymlinks(got.Dir)
			wantArgs := []string{literal}
			wantInput := FormatPrompt(messages)
			if argument {
				wantArgs = append(wantArgs, wantInput)
				wantInput = ""
			}
			if gotDir != wantDir || got.Value != "override with 空格" || got.Stdin != wantInput || !reflect.DeepEqual(got.Args, wantArgs) {
				t.Fatalf("helper input = %+v", got)
			}
			if _, err := os.Stat(filepath.Join(dir, "must-not-exist")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("literal shell argument executed: %v", err)
			}
		})
	}
}

func TestCLIAdapterPreservesBoundedFailureDetailsAndValidUTF8(t *testing.T) {
	message, err := helperCLI("failure").Call(context.Background(), []Message{{Text: "hello"}})
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 23 || message.Text != "partial answer" {
		t.Fatalf("failed call = %+v, %v", message, err)
	}
	if !strings.Contains(err.Error(), "last diagnostic") || !utf8.ValidString(err.Error()) || len(err.Error()) > MaxStderrBytes+512 {
		t.Fatalf("failure diagnostic bytes=%d, valid=%v", len(err.Error()), utf8.ValidString(err.Error()))
	}
	message, err = helperCLI("invalid-utf8").Call(context.Background(), nil)
	if err != nil || message.Text != "ok\n" || !utf8.ValidString(message.Text) {
		t.Fatalf("UTF8 result = %+v, %v", message, err)
	}
	message, err = helperCLI("overflow").Call(context.Background(), nil)
	if !errors.Is(err, ErrOutputTooLarge) || len(message.Text) != MaxOutputBytes {
		t.Fatalf("overflow bytes=%d, error=%v", len(message.Text), err)
	}
}

func TestCLIAdapterTimeoutAndCancellation(t *testing.T) {
	adapter := helperCLI("wait")
	adapter.Timeout = 200 * time.Millisecond
	started := time.Now()
	_, err := adapter.Call(context.Background(), nil)
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(started) > 3*time.Second {
		t.Fatalf("timeout duration=%v, error=%v", time.Since(started), err)
	}

	adapter = helperCLI("wait")
	ready := filepath.Join(t.TempDir(), "ready")
	adapter.Env["FIVELEAF_ROUNDTABLE_TEST_READY"] = ready
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := adapter.Call(ctx, nil)
		done <- err
	}()
	waitForFile(t, ready)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled call = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cancel did not terminate the command")
	}
}

func TestCLIAdapterRejectsInvalidConfiguration(t *testing.T) {
	for _, adapter := range []CLIAdapter{
		{},
		{Command: "bad\x00command"},
		{Command: os.Args[0], Args: []string{"bad\x00argument"}},
		{Command: os.Args[0], Env: map[string]string{"bad=name": "value"}},
		{Command: os.Args[0], Env: map[string]string{"name": "bad\x00value"}},
	} {
		if _, err := adapter.Call(context.Background(), nil); err == nil {
			t.Fatalf("invalid adapter accepted: %+v", adapter)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := helperCLI("echo").Call(ctx, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("already-cancelled call = %v", err)
	}
}

package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUpdatePolicyFetch(t *testing.T) {
	for _, test := range []struct {
		name    string
		status  int
		body    string
		minimum string
		wantErr bool
	}{
		{name: "require", status: 200, body: `{"minimumVersion":"0.21.0"}`, minimum: "0.21.0"},
		{name: "relax", status: 200, body: `{"minimumVersion":""}`},
		{name: "missing", status: 404},
		{name: "server error", status: 503, wantErr: true},
		{name: "malformed", status: 200, body: `{`, wantErr: true},
		{name: "missing field", status: 200, body: `{}`, wantErr: true},
		{name: "null field", status: 200, body: `{"minimumVersion":null}`, wantErr: true},
		{name: "incomplete version", status: 200, body: `{"minimumVersion":"0.21"}`, wantErr: true},
		{name: "prerelease", status: 200, body: `{"minimumVersion":"0.21.0-beta.1"}`, wantErr: true},
		{name: "oversized", status: 200, body: strings.Repeat(" ", updatePolicyLimit+1), wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.WriteHeader(test.status)
				fmt.Fprint(writer, test.body)
			}))
			defer server.Close()
			minimum, err := fetchUpdateMinimum(t.Context(), server.Client(), server.URL)
			if (err != nil) != test.wantErr || minimum != test.minimum {
				t.Fatalf("fetch returned %q, %v, want %q and error=%t", minimum, err, test.minimum, test.wantErr)
			}
		})
	}
}

func TestUpdatePolicyMinimum(t *testing.T) {
	for _, test := range []struct {
		name, current, available, minimum string
		required                          bool
	}{
		{"matches floor", "0.20.9", "0.21.0", "0.21.0", true},
		{"newer target", "0.20.9", "0.22.0", "0.21.0", true},
		{"numeric ordering", "0.9.1", "v0.11.0", "0.10.0", true},
		{"prerelease before floor", "0.21.0-rc.1", "0.21.0", "0.21.0", true},
		{"already supported", "0.21.0", "0.22.0", "0.21.0", false},
		{"no downgrade", "0.22.0", "0.20.0", "0.21.0", false},
		{"target below floor", "0.19.0", "0.20.0", "0.21.0", false},
		{"target is prerelease of floor", "0.20.0", "0.21.0-beta.2", "0.21.0", false},
		{"no policy", "0.20.0", "0.21.0", "", false},
		{"no target", "0.20.0", "", "0.21.0", false},
		{"invalid running version", "dev", "0.21.0", "0.21.0", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			minimum := requiredUpdateMinimum(test.current, test.available, test.minimum)
			if (minimum != "") != test.required {
				t.Fatalf("required minimum = %q, want required=%t", minimum, test.required)
			}
		})
	}
}

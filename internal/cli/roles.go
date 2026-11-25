package cli

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"fiveleaf/internal/settings"
)

type RoleRequest struct {
	Action string          `json:"action"`
	Bundle json.RawMessage `json:"bundle,omitempty"`
	source string
}

func parseRoleRequest(args []string, cwd string) (*RoleRequest, error) {
	if len(args) == 0 {
		return &RoleRequest{Action: "list"}, nil
	}
	request := &RoleRequest{Action: args[0]}
	switch args[0] {
	case "--help", "-h":
		return nil, ErrHelp
	case "list", "export", "socket":
		if len(args) == 1 {
			return request, nil
		}
	case "import":
		if len(args) == 1 || len(args) == 2 && args[1] == "-" {
			return request, nil
		}
		if len(args) == 2 && args[1] != "" && !strings.HasPrefix(args[1], "-") {
			request.source = args[1]
			if !filepath.IsAbs(request.source) {
				request.source = filepath.Join(cwd, request.source)
			}
			return request, nil
		}
	}
	return nil, errors.New("use ff roles list, ff roles export, ff roles import [<file>|-], or ff roles socket")
}

func (request *RoleRequest) readBundle(input io.Reader) error {
	if request.source != "" {
		file, err := os.Open(request.source)
		if err != nil {
			return err
		}
		defer file.Close()
		input = file
	} else if input == nil {
		input = os.Stdin
	}
	raw, err := io.ReadAll(io.LimitReader(input, settings.MaxAgentConfigurationSize+1))
	if err != nil {
		return err
	}
	if len(raw) > settings.MaxAgentConfigurationSize {
		return errors.New("role bundle is too large")
	}
	if !json.Valid(raw) {
		return errors.New("role bundle must be valid JSON")
	}
	request.Bundle = raw
	return nil
}

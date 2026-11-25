package acp

import (
	"context"
	"os"

	sdk "github.com/coder/acp-go-sdk"
)

func DiscoverModels(ctx context.Context, config Config, workspace string) ([]ModelChoice, error) {
	if workspace == "" {
		dir, err := os.MkdirTemp("", "fiveleaf-models-*")
		if err != nil {
			return nil, err
		}
		defer os.RemoveAll(dir)
		workspace = dir
	}
	client, err := Start(ctx, config, workspace, catalogHandler{})
	if err != nil {
		return nil, err
	}
	defer client.Close()
	if _, err := client.initialize(ctx, sdk.ClientCapabilities{}); err != nil {
		return nil, err
	}
	session, err := client.NewSession(ctx)
	if err != nil {
		return nil, err
	}
	state, _ := client.ModelState(string(session.SessionId))
	return state.Available, nil
}

type catalogHandler struct{}

func (catalogHandler) Update(Update) {}
func (catalogHandler) RequestPermission(context.Context, PermissionRequest) (PermissionOutcome, error) {
	return CancelledPermission(), nil
}
func (catalogHandler) ReadTextFile(context.Context, ReadRequest) (string, error) {
	return "", os.ErrPermission
}
func (catalogHandler) WriteTextFile(context.Context, WriteRequest) error { return os.ErrPermission }

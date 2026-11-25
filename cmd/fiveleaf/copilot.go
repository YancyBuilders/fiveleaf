package main

import (
	"context"
	"errors"
	"reflect"
	"time"

	"fiveleaf/internal/acp"
	"fiveleaf/internal/remote"
	"fiveleaf/internal/roundtable"
	"fiveleaf/internal/settings"
	"fiveleaf/locales"
)

type copilotRequest struct {
	selection settings.CopilotConfig
	provider  agentProvider
	folder    folderKey
	valid     func() bool
	cancel    context.CancelFunc
	preview   bool
}

func (request *copilotRequest) current(a *app, providers []agentProvider) bool {
	return a.folderKey() == request.folder && a.settings.Copilot == request.selection && request.valid() && request.sameProvider(providers)
}

func (request *copilotRequest) sameProvider(providers []agentProvider) bool {
	return reflect.DeepEqual(findChatProvider(request.selection.Agent, providers), request.provider)
}

func (a *app) cancelCopilot() {
	a.dismissInlineCompletion()
	if request := a.copilot; request != nil {
		a.copilot = nil
		request.cancel()
		if request.preview {
			a.closePluginPicker(-1)
		}
	}
}

func (a *app) copilotError(err error) {
	a.tell(note{message: err.Error(), label: locales.CopilotConfigure(), action: func() {
		a.host.showSettings(settings.CatAgent, "copilot")
	}})
}

func (a *app) copilotProvider() (agentProvider, error) {
	selection := a.settings.Copilot
	provider := findChatProvider(selection.Agent, a.agentProviders())
	switch {
	case selection.Agent == "":
		return provider, errors.New(locales.CopilotChooseAgent())
	case provider.config.Command == "" || provider.config.Disabled:
		return provider, errors.New(locales.CopilotUnavailable(selection.Agent))
	}
	if err := selection.Validate(); err != nil {
		return provider, err
	}
	if err := provider.config.Validate(); err != nil {
		return provider, err
	}
	if !provider.presetModelCompatible(selection.Model) {
		return provider, errors.New(locales.CopilotCLIModel())
	}
	provider.config = cloneAgentServer(provider.config)
	return provider, nil
}

func (a *app) runCopilot(valid func() bool, work func(context.Context, *copilotRequest) (string, error), accept func(*copilotRequest, string)) {
	selection := a.settings.Copilot
	if selection.Agent != "" && (a.conn != nil || a.root == "" || remote.Is(a.root)) {
		a.fail("%s", locales.AgentRemoteUnavailable())
		return
	}
	provider, err := a.copilotProvider()
	if err != nil {
		a.copilotError(err)
		return
	}
	a.cancelCopilot()
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(provider.config.ReplyTimeoutSeconds())*time.Second)
	request := &copilotRequest{selection: selection, provider: provider, folder: a.folderKey(), valid: valid, cancel: cancel}
	a.copilot = request
	a.tell(note{message: locales.CopilotWorking()})
	go func() {
		defer cancel()
		text, err := work(ctx, request)
		a.post(func() {
			if a.copilot != request {
				return
			}
			if !request.current(a, a.agentProviders()) {
				a.cancelCopilot()
				return
			}
			switch {
			case err != nil:
				a.copilotError(err)
			case text == "":
				a.tell(note{message: locales.CompletionNone()})
			default:
				accept(request, text)
			}
			if !request.preview {
				a.copilot = nil
			}
		})
	}()
}

func (request *copilotRequest) generate(ctx context.Context, prompt string) (string, error) {
	root := request.folder.root
	provider, model := request.provider, request.selection.Model
	if provider.config.EffectiveProtocol() != "cli" {
		return acp.GenerateText(ctx, agentACPConfig(provider), root, model, prompt)
	}
	effective, err := agentRoleCLIProvider(provider, model)
	if err != nil {
		return "", err
	}
	reply, err := cliAgentAdapter(root, effective).Call(ctx, []roundtable.Message{{Role: "user", Text: prompt}})
	return reply.Text, err
}

package main

import (
	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

type agentRolePreset struct {
	id           string
	name, prompt func() string
	icon         *ui.SVG
}

var agentRolePresets = [...]agentRolePreset{
	{"explore", locales.AgentRoleExplorer, locales.AgentRoleExplorerPrompt, iconFileSearch},
	{"plan", locales.AgentRolePlanner, locales.AgentRolePlannerPrompt, iconClipboardList},
	{"implement", locales.AgentRoleImplementer, locales.AgentRoleImplementerPrompt, iconBraces},
	{"test", locales.AgentRoleTester, locales.AgentRoleTesterPrompt, iconCircleCheck},
	{"review", locales.AgentRoleReviewer, locales.AgentRoleReviewerPrompt, iconPullRequest},
	{"diagnose", locales.AgentRoleDiagnostician, locales.AgentRoleDiagnosticianPrompt, iconBug},
	{"document", locales.AgentRoleDocWriter, locales.AgentRoleDocWriterPrompt, iconFileText},
	{"requirements", locales.AgentRoleRequirementsAnalyst, locales.AgentRoleRequirementsAnalystPrompt, iconScrollText},
	{"interaction", locales.AgentRoleInteractionDesigner, locales.AgentRoleInteractionDesignerPrompt, iconColumns},
	{"compatibility", locales.AgentRoleCompatibilityMaintainer, locales.AgentRoleCompatibilityMaintainerPrompt, iconPlug},
	{"moderate", locales.AgentRoleRoundtableModerator, locales.AgentRoleRoundtableModeratorPrompt, iconMessageText},
}

func agentPresetProvider(providers []agentProvider, assigned, model string) agentProvider {
	if assigned != "" {
		return findChatProvider(assigned, providers)
	}
	var fallback agentProvider
	for _, provider := range providers {
		if provider.config.Disabled {
			continue
		}
		if provider.config.Validate() == nil && provider.presetModelCompatible(model) {
			return provider
		}
		if fallback.id == "" {
			fallback = provider
		}
	}
	return fallback
}

package main

import (
	"fiveleaf/locales"
	"net/url"
	"runtime"
	"strings"
)

// repoURL is set by the release build to the fork's repository.
var repoURL string

// reportProblem opens a new issue of FiveLeaf's repository in the browser,
// titled title, with details and what any report needs: the version and
// the system. The person reads it and sends it, or not.
func (a *app) reportProblem(title, details string) {
	if target := reportURL(title, details); target != "" {
		a.services.OpenURL(target)
		return
	}
	a.services.WriteClipboard(strings.TrimSpace(title + "\n\n" + reportBody(details)))
	a.tell(note{message: locales.ReportCopied()})
}

// reportURL is the address of a new issue of title, its body details and
// the version and system FiveLeaf runs on.
func reportURL(title, details string) string {
	if repoURL == "" {
		return ""
	}
	q := url.Values{"title": {title}, "body": {reportBody(details)}}
	return strings.TrimRight(repoURL, "/") + "/issues/new?" + q.Encode()
}

func reportBody(details string) string {
	body := "FiveLeaf " + appVersion() + " on " + runtime.GOOS + "/" + runtime.GOARCH + "\n\n"
	if details != "" {
		body += details + "\n\n"
	}
	body += "What did you do, and what did you expect to happen?\n"
	return body
}

// reportFromMenu is Help → Report a Problem…: an issue with nothing filled
// in but the version and the system.
func (a *app) reportFromMenu() { a.reportProblem("", "") }

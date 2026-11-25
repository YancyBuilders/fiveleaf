package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"golang.org/x/mod/semver"
)

const updatePolicyLimit = 4096

var updatePolicyURL string

var updatePolicyClient = &http.Client{
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

func fetchUpdateMinimum(ctx context.Context, client *http.Client, endpoint string) (string, error) {
	if endpoint == "" {
		return "", nil
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	if request.URL.Scheme != "https" || request.URL.Hostname() == "" || request.URL.User != nil {
		return "", errors.New("the update policy needs an HTTPS URL")
	}
	request.Header.Set("Cache-Control", "no-cache")
	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case http.StatusNotFound:
		return "", nil
	case http.StatusOK:
	default:
		return "", fmt.Errorf("update policy returned %s", response.Status)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, updatePolicyLimit+1))
	if err != nil {
		return "", err
	}
	if len(data) > updatePolicyLimit {
		return "", errors.New("update policy is too large")
	}
	var policy struct {
		Minimum *string `json:"minimumVersion"`
	}
	if err := json.Unmarshal(data, &policy); err != nil {
		return "", err
	}
	if policy.Minimum == nil {
		return "", errors.New("update policy has no minimumVersion")
	}
	minimum := *policy.Minimum
	if minimum != "" {
		tag := "v" + minimum
		if semver.Canonical(tag) != tag || semver.Prerelease(tag) != "" {
			return "", errors.New("minimumVersion must be a stable version in major.minor.patch form")
		}
	}
	return minimum, nil
}

func requiredUpdateMinimum(current, available, minimum string) string {
	current = "v" + strings.TrimPrefix(current, "v")
	available = "v" + strings.TrimPrefix(available, "v")
	floor := "v" + minimum
	if !semver.IsValid(floor) || !semver.IsValid(current) || !semver.IsValid(available) ||
		semver.Compare(current, floor) >= 0 || semver.Compare(available, floor) < 0 {
		return ""
	}
	return minimum
}

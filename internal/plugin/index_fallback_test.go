package plugin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fiveleaf/extensions"
)

type indexHTTPStatus int

func (status indexHTTPStatus) RoundTrip(req *http.Request) (*http.Response, error) {
	code := int(status)
	return &http.Response{Request: req, StatusCode: code, Status: fmt.Sprintf("%d %s", code, http.StatusText(code)), Body: http.NoBody}, nil
}

func TestIndexFallbackRequiresOfficialServerError(t *testing.T) {
	source := IndexURL
	t.Cleanup(func() { IndexURL = source })
	for _, test := range []struct {
		url    string
		status int
	}{
		{source, http.StatusForbidden},
		{"https://custom.invalid/index.json", http.StatusServiceUnavailable},
	} {
		IndexURL = test.url
		entries, err := FetchIndex(t.Context(), &http.Client{Transport: indexHTTPStatus(test.status)})
		if err == nil || len(entries) != 0 {
			t.Errorf("%s returned %d: got %v, %v", test.url, test.status, entries, err)
		}
	}
}

func TestIndexNeedsAPluginList(t *testing.T) {
	for _, body := range []string{`null`, `{}`, `{"error":"unavailable"}`, `{"plugins":null}`, `[]`, `{"plugins":[]}`} {
		t.Run(body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				fmt.Fprint(writer, body)
			}))
			defer server.Close()
			previous := IndexURL
			IndexURL = server.URL
			t.Cleanup(func() { IndexURL = previous })
			entries, err := FetchIndex(t.Context(), server.Client())
			if body == `[]` || body == `{"plugins":[]}` {
				if err != nil || entries == nil || len(entries) != 0 {
					t.Fatalf("valid empty list returned %v, %v", entries, err)
				}
			} else if err == nil || entries != nil {
				t.Fatalf("response without a list returned %v, %v", entries, err)
			}
		})
	}
}

func TestMarketplaceDuringIndexOutage(t *testing.T) {
	previous := http.DefaultClient
	http.DefaultClient = &http.Client{Transport: indexHTTPStatus(http.StatusServiceUnavailable)}
	t.Cleanup(func() { http.DefaultClient = previous })
	data, err := os.ReadFile("../../extensions/index.json")
	if err != nil {
		t.Fatal(err)
	}
	var catalog []IndexEntry
	if err := json.Unmarshal(data, &catalog); err != nil || len(catalog) == 0 {
		t.Fatalf("bundled catalog: %v, %d entries", err, len(catalog))
	}
	dirs := Dirs{Plugins: t.TempDir()}
	var output, warnings strings.Builder
	if code := RunCLI([]string{"search"}, dirs, &output, &warnings); code != 0 || !strings.Contains(warnings.String(), "503") {
		t.Fatalf("search during outage = %d, %q, %q", code, &output, &warnings)
	}
	for _, entry := range catalog {
		if !strings.Contains(output.String(), entry.Name) {
			t.Errorf("search during outage omitted %s", entry.Name)
		}
	}
	entry := catalog[0]
	repository, _, _ := strings.Cut(entry.Repository, "#")
	release, ok := entry.Newest()
	if !ok {
		t.Fatalf("%s has no release", entry.Name)
	}
	fixture := gitRepo(t, map[string]string{filepath.Join(release.Path, ManifestFile): `{"name":"` + entry.Name + `","version":"` + release.Version + `"}`})
	fixtureCommit := sourceGit(t, fixture, "rev-parse", "HEAD")
	catalog[0].Versions[0].Commit = fixtureCommit
	fallback, err := json.Marshal(catalog)
	if err != nil {
		t.Fatal(err)
	}
	previousCatalog := extensions.CatalogJSON
	extensions.CatalogJSON = string(fallback)
	t.Cleanup(func() { extensions.CatalogJSON = previousCatalog })
	localURL := (&url.URL{Scheme: "file", Path: filepath.ToSlash(fixture)}).String()
	t.Setenv("GIT_ALLOW_PROTOCOL", "file")
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "url."+localURL+".insteadOf")
	t.Setenv("GIT_CONFIG_VALUE_0", repository)
	if manifest, err := InstallSpec(t.Context(), dirs.Plugins, entry.Name, nil); err != nil || manifest.Name != entry.Name {
		t.Fatalf("install by name during outage = %v, %v", manifest, err)
	}
	output.Reset()
	warnings.Reset()
	cliDirs := Dirs{Plugins: t.TempDir()}
	if code := RunCLI([]string{"install", entry.Name}, cliDirs, &output, &warnings); code != 0 || !strings.Contains(output.String(), "Installed "+entry.Name) {
		t.Fatalf("CLI install during outage = %d, %q, %q", code, &output, &warnings)
	}
	if _, err := InstallSpec(t.Context(), dirs.Plugins, "only-in-live-index", nil); err == nil || !strings.Contains(err.Error(), "503") {
		t.Fatalf("name absent from bundled catalog during outage = %v", err)
	}
}

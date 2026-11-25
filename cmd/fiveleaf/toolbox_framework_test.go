package main

import (
	"bytes"
	"context"
	"mime"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestToolboxFrameworkBuildInEditor(t *testing.T) {
	for _, framework := range []string{"vue", "react"} {
		t.Run(framework, func(t *testing.T) {
			variable := "FIVELEAF_TEST_TOOLBOX_" + strings.ToUpper(framework)
			folder := os.Getenv(variable)
			if folder == "" {
				t.Skipf("set %s to a built ff plugin new --tool=%s directory", variable, framework)
			}
			harness := newHarness(t, false)
			host := withPlugins(harness, nil)
			manifest, err := host.reg.Dirs.LinkDev(folder)
			if err != nil {
				t.Fatal(err)
			}
			host.reg.Load()
			harness.a.openToolboxTab(manifest.Name)
			tab := harness.a.activeTab()
			if tab == nil || tab.tool == nil || tab.tool.toolbox == nil || tab.tool.toolbox.provider != host.reg.Plugin(manifest.Name) {
				t.Fatal("the linked plugin did not open in its editor tab")
			}
			route := toolboxURL(tab.tool.toolbox.provider)
			handler := toolboxSurfaceFiles(toolboxFiles())
			page := filepath.Join(folder, filepath.FromSlash(manifest.Contributes.Toolbox.Page))
			get := func(asset string) *httptest.ResponseRecorder {
				t.Helper()
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, httptest.NewRequest("GET", route+asset, nil))
				if response.Code != http.StatusOK {
					t.Fatalf("%s returned %d", asset, response.Code)
				}
				return response
			}
			source, err := os.ReadFile(page)
			if err != nil {
				t.Fatal(err)
			}
			document := get("")
			hook := []byte(`<script src="` + toolboxBridgePath + `"></script>`)
			if bytes.Count(document.Body.Bytes(), hook) != 1 || !bytes.Equal(bytes.Replace(document.Body.Bytes(), hook, nil, 1), source) {
				t.Fatal("the editor changed the generated HTML beyond adding the Toolbox bridge")
			}
			linkedFiles := regexp.MustCompile(`(?:src|href)="\./([^"?#]+\.(js|css))"`).FindAllStringSubmatch(string(source), -1)
			seen := map[string]bool{}
			for _, file := range linkedFiles {
				name, extension := file[1], file[2]
				response := get(name)
				typeName, _, err := mime.ParseMediaType(response.Header().Get("Content-Type"))
				if err != nil || extension == "css" && typeName != "text/css" || extension == "js" && typeName != "text/javascript" && typeName != "application/javascript" {
					t.Fatalf("%s has content type %q", name, typeName)
				}
				generated, err := os.ReadFile(filepath.Join(filepath.Dir(page), filepath.FromSlash(name)))
				if err != nil || !bytes.Equal(response.Body.Bytes(), generated) {
					t.Fatalf("served asset differs from %s: %v", name, err)
				}
				seen[extension] = true
			}
			if !seen["css"] || !seen["js"] {
				t.Fatalf("missing stylesheet or script in %s", page)
			}
			browser := os.Getenv("FIVELEAF_TEST_CHROMIUM")
			if browser == "" {
				t.Log("set FIVELEAF_TEST_CHROMIUM for the browser interaction check")
				return
			}
			mux := http.NewServeMux()
			mux.Handle("/original/", http.StripPrefix("/original/", http.FileServer(http.Dir(filepath.Dir(page)))))
			mux.HandleFunc("GET /compare", func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("Content-Type", "text/html")
				writer.Write([]byte(frameworkPageComparison))
			})
			mux.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
				request.Host = manifest.Name + ".localhost"
				handler.ServeHTTP(writer, request)
			})
			server := httptest.NewServer(mux)
			defer server.Close()
			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
			defer cancel()
			dom, err := exec.CommandContext(ctx, browser, "--headless", "--no-first-run", "--disable-background-networking", "--user-data-dir="+t.TempDir(), "--virtual-time-budget=12000", "--dump-dom", server.URL+"/compare").Output()
			if err != nil || !bytes.Contains(dom, []byte(`<output id="result">pass</output>`)) {
				t.Fatalf("rendering %s: %v\n%s", manifest.Name, err, dom)
			}
		})
	}
}

const frameworkPageComparison = `<!doctype html><meta charset="utf-8">
<iframe id="original" src="/original/"></iframe>
<iframe id="toolbox" src="/" sandbox="allow-scripts allow-same-origin allow-forms allow-downloads"></iframe>
<output id="result">pending</output>
<script>
addEventListener('load', async () => {
  const result = document.querySelector('#result');
  try {
    const inspect = async frame => {
      const view = frame.contentWindow, doc = view.document;
      const wait = () => new Promise(done => setTimeout(done, 20));
      for (let attempt = 0; !doc.querySelector('output') && attempt < 80; attempt++) await wait();
      const input = doc.querySelector('textarea'), output = doc.querySelector('output');
      if (output?.textContent !== 'Characters: 0') throw Error('component failed to mount');
      for (const [value, count] of [['A🌿中', 3], ['', 0]]) {
        Object.getOwnPropertyDescriptor(view.HTMLTextAreaElement.prototype, 'value').set.call(input, value);
        input.dispatchEvent(new view.Event('input', {bubbles: true}));
        for (let attempt = 0; output.textContent !== 'Characters: ' + count && attempt < 80; attempt++) await wait();
        if (output.textContent !== 'Characters: ' + count) throw Error('unexpected counter: ' + output.textContent);
      }
      const main = doc.querySelector('main'), style = view.getComputedStyle(main);
      if (style.display !== 'grid' || !doc.styleSheets.length) throw Error('component stylesheet is missing');
      return [main.outerHTML, style.display, style.gap, style.maxWidth, style.padding, view.getComputedStyle(input).font];
    };
    const [original, embedded] = await Promise.all([...document.querySelectorAll('iframe')].map(inspect));
    if (JSON.stringify(original) !== JSON.stringify(embedded)) throw Error('embedded component or styles differ');
    result.textContent = 'pass';
  } catch (error) { result.textContent = error.message; }
});
</script>`

package importmap

import (
	"bytes"
	"context"
	"html/template"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/donseba/go-importmap/library"
)

// countingProvider counts the packages it is asked for.
type countingProvider struct {
	fixtureProvider
	calls map[string]*atomic.Int32
}

func (p countingProvider) FetchPackageFiles(ctx context.Context, name, version string) (library.Files, string, error) {
	p.calls[name].Add(1)

	if version == "" {
		version = "9.9.9"
	}

	return p.fixtureProvider.FetchPackageFiles(ctx, name, version)
}

func TestCacheOrFetchFetchesOnlyMissingPackages(t *testing.T) {
	root := t.TempDir()
	server := assetServer(t)
	provider := countingProvider{
		fixtureProvider: fixtureProvider{baseURL: server.URL, paths: map[string][]string{
			"one": {"one.js"},
			"two": {"two.js"},
		}},
		calls: map[string]*atomic.Int32{"one": {}, "two": {}},
	}

	one := library.Package{Name: "one", Version: "1.0.0"}
	two := library.Package{Name: "two", Version: "2.0.0"}

	if err := NewDefaults().RootDir(root).WithProvider(provider).WithPackage(one).Fetch(t.Context()); err != nil {
		t.Fatal(err)
	}

	im := NewDefaults().RootDir(root).WithProvider(provider).WithPackages([]library.Package{one, two})
	if err := im.CacheOrFetch(t.Context()); err != nil {
		t.Fatal(err)
	}

	if got := provider.calls["one"].Load(); got != 1 {
		t.Errorf("cached package fetched again: %d calls", got)
	}

	if got := provider.calls["two"].Load(); got != 1 {
		t.Errorf("missing package fetched %d times", got)
	}

	if im.Structure.Imports["one.js"] != "/assets/one/one.js" || im.Structure.Imports["two.js"] != "/assets/two/two.js" {
		t.Errorf("imports = %v", im.Structure.Imports)
	}
}

func TestCacheOrFetchAppliesRawImports(t *testing.T) {
	im := NewDefaults().RootDir(t.TempDir()).WithProvider(offlineProvider{}).WithPackages([]library.Package{{
		Name: "htmx", Version: "4.0.0",
		Require: library.Includes{{Raw: "https://cdn.example/htmx.esm.js", As: "htmx"}},
	}})

	if err := im.CacheOrFetch(t.Context()); err != nil {
		t.Fatal(err)
	}

	if got := im.Structure.Imports["htmx"]; got != "https://cdn.example/htmx.esm.js" {
		t.Fatalf("raw import = %q", got)
	}
}

func TestFetchKeepsResolvedVersion(t *testing.T) {
	server := assetServer(t)
	provider := countingProvider{
		fixtureProvider: fixtureProvider{baseURL: server.URL, paths: map[string][]string{"demo": {"demo.js"}}},
		calls:           map[string]*atomic.Int32{"demo": {}},
	}
	packages := []library.Package{{Name: "demo"}}

	im := NewDefaults().RootDir(t.TempDir()).WithProvider(provider).WithPackages(packages)
	if err := im.Fetch(t.Context()); err != nil {
		t.Fatal(err)
	}

	if got := im.packages[0].Version; got != "9.9.9" {
		t.Errorf("resolved version = %q", got)
	}

	if packages[0].Version != "" {
		t.Errorf("caller's packages changed: %q", packages[0].Version)
	}

	// The second run finds the cache of the resolved version.
	if err := im.CacheOrFetch(t.Context()); err != nil {
		t.Fatal(err)
	}

	if got := provider.calls["demo"].Load(); got != 1 {
		t.Errorf("provider asked %d times", got)
	}
}

func TestRenderIsOrderedAndEscaped(t *testing.T) {
	im := New()
	im.Structure.Styles = map[string]string{
		"c": "/c.css",
		"a": `/a.css?x="1"&y=<2>`,
		"b": "/b.css",
	}
	im.WithImport("app", "/app.js?a=1&b=2", "")

	want := `<link rel="stylesheet" href="/a.css?x=&#34;1&#34;&amp;y=&lt;2&gt;"/>
<link rel="stylesheet" href="/b.css"/>
<link rel="stylesheet" href="/c.css"/>
<script type="importmap">
{
  "imports": {
    "app": "/app.js?a=1\u0026b=2"
  }
}
</script>`

	for range 20 {
		got, err := im.Render()
		if err != nil {
			t.Fatal(err)
		}

		if string(got) != want {
			t.Fatalf("render =\n%q\nwant\n%q", got, want)
		}
	}

	styles, err := im.Styles()
	if err != nil {
		t.Fatal(err)
	}

	if !strings.HasPrefix(
		string(styles),
		`<link rel="stylesheet" href="/a.css?x=&#34;1&#34;&amp;y=&lt;2&gt;"><link rel="stylesheet" href="/b.css">`,
	) {
		t.Fatalf("styles = %s", styles)
	}
}

func TestShimIsOptIn(t *testing.T) {
	im := NewDefaults().WithImport("app", "/app.js", "")
	if im.Shim() != "" {
		t.Fatalf("default shim = %q", im.Shim())
	}

	out, _ := im.Render()
	if strings.Contains(string(out), "<script async") {
		t.Fatalf("render loads a shim: %s", out)
	}

	out, _ = im.ShimPath("/assets/es-module-shims.js").Render()
	if !strings.Contains(string(out), `<script async src="/assets/es-module-shims.js"></script>`) {
		t.Fatalf("render misses the shim: %s", out)
	}
}

func TestLocalEntriesFollowURLFunc(t *testing.T) {
	im := New().
		WithFile("app", "app/main.js").
		WithFile("theme", "/app/theme.css").
		WithImport("partial-sse", "/admin/presence/client.js", "").
		ModulePreload("app", "missing")

	if got := im.Structure.Imports["app"]; got != "/assets/app/main.js" {
		t.Errorf("default URL = %q", got)
	}

	im.WithURLFunc(func(file string) string {
		return "/admin/assets/" + file
	})

	if got := im.Structure.Imports["app"]; got != "/admin/assets/app/main.js" {
		t.Errorf("routed import = %q", got)
	}

	if got := im.Structure.Styles["theme"]; got != "/admin/assets/app/theme.css" {
		t.Errorf("routed style = %q", got)
	}

	if got := im.Structure.Imports["partial-sse"]; got != "/admin/presence/client.js" {
		t.Errorf("fixed import = %q", got)
	}

	if got := im.URL("editor/loader.js"); got != "/admin/assets/editor/loader.js" {
		t.Errorf("URL = %q", got)
	}

	out, err := im.Render()
	if err != nil {
		t.Fatal(err)
	}

	if !strings.HasSuffix(string(out), "</script>\n<link rel=\"modulepreload\" href=\"/admin/assets/app/main.js\"/>") {
		t.Fatalf("render = %s", out)
	}

	out, _ = im.ModulePreload("partial-sse", "app").Render()
	if strings.Count(string(out), "modulepreload") != 2 {
		t.Fatalf("preloads = %s", out)
	}

	// A fixed import replaces a local one of the same name.
	im.WithImport("app", "https://cdn.example/app.js", "").WithURLFunc(nil)

	if got := im.Structure.Imports["app"]; got != "https://cdn.example/app.js" {
		t.Errorf("replaced import = %q", got)
	}
}

func TestFuncMap(t *testing.T) {
	im := New().WithFile("app", "app.js")
	tmpl := template.Must(
		template.New("head").
			Funcs(im.FuncMap()).
			Parse(`{{importmap}}<script type="module" src="{{importmap_url "app.js"}}"></script>`),
	)

	var out bytes.Buffer
	if err := tmpl.Execute(&out, nil); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out.String(), `<script type="importmap">`) ||
		!strings.HasSuffix(out.String(), `<script type="module" src="/assets/app.js"></script>`) {
		t.Fatalf("template = %s", out.String())
	}
}

func TestScriptAndStylesheet(t *testing.T) {
	im, err := FromFS(localFS(t), "vendor/importmap.lock.json")
	if err != nil {
		t.Fatal(err)
	}

	im.WithURLFunc(func(file string) string { return "/admin/assets/" + file })
	manifest := im.Manifest()

	script, err := im.Script("editor/editor.js", "type", "module", "data-picker", `/a?b=1&c="2"`, "defer", "")
	if err != nil {
		t.Fatal(err)
	}

	want := `<script src="/admin/assets/editor/editor.js?v=` + manifest.Version("editor/editor.js") +
		`" type="module" data-picker="/a?b=1&amp;c=&#34;2&#34;" defer></script>`
	if string(script) != want {
		t.Errorf("Script without integrity =\n%s\nwant\n%s", script, want)
	}

	im.WithRenderOptions(WithIntegrity())

	script, err = im.Script("/editor/editor.js")
	if err != nil {
		t.Fatal(err)
	}

	want = `<script src="/admin/assets/editor/editor.js?v=` + manifest.Version("editor/editor.js") +
		`" integrity="` + manifest.Integrity("editor/editor.js") +
		`" crossorigin="anonymous"></script>`
	if string(script) != want {
		t.Errorf("Script =\n%s\nwant\n%s", script, want)
	}

	style, err := im.Stylesheet("icons.svg", "media", "print")
	if err != nil {
		t.Fatal(err)
	}

	want = `<link rel="stylesheet" href="/admin/assets/icons.svg?v=` + manifest.Version("icons.svg") +
		`" integrity="` + manifest.Integrity("icons.svg") +
		`" crossorigin="anonymous" media="print">`
	if string(style) != want {
		t.Errorf("Stylesheet =\n%s\nwant\n%s", style, want)
	}

	// The template functions are the same methods.
	funcs := im.FuncMap()
	if _, ok := funcs["importmap_script"].(func(string, ...string) (template.HTML, error)); !ok {
		t.Error("importmap_script is missing")
	}

	if _, ok := funcs["importmap_style"].(func(string, ...string) (template.HTML, error)); !ok {
		t.Error("importmap_style is missing")
	}

	if _, err := im.Script("app.js", "type"); err == nil {
		t.Error("an attribute without a value did not fail")
	}

	if _, err := im.Script("app.js", "src", "/x.js"); err == nil {
		t.Error("src among the attributes did not fail")
	}

	// A file without a manifest gets no integrity.
	plain, err := New().WithRenderOptions(WithIntegrity()).Script("app.js")
	if err != nil || string(plain) != `<script src="/assets/app.js"></script>` {
		t.Errorf("Script without a manifest = %s, %v", plain, err)
	}
}

// TestWithImportIntegrity checks that a module mapped with its own
// integrity has it in the map's integrity section and on its preload
// link, with WithIntegrity only, and that an empty integrity maps the URL
// alone.
func TestWithImportIntegrity(t *testing.T) {
	im := New().
		WithImport("client", "/lib/client.js?v=1", "sha384-abc").
		WithImport("plain", "/lib/plain.js", "").
		ModulePreload("client")

	plain, err := im.Render()
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(string(plain), "sha384-abc") {
		t.Fatalf("integrity without WithIntegrity:\n%s", plain)
	}

	html, err := im.RenderWith(WithIntegrity())
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		`"client": "/lib/client.js?v=1"`,
		`"plain": "/lib/plain.js"`,
		`"integrity": {` + "\n" + `    "/lib/client.js?v=1": "sha384-abc"` + "\n" + `  }`,
		`<link rel="modulepreload" href="/lib/client.js?v=1" integrity="sha384-abc"/>`,
	} {
		if !strings.Contains(string(html), want) {
			t.Errorf("missing %s in\n%s", want, html)
		}
	}
}

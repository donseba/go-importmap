package importmap

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"html/template"
	"regexp"
	"strings"
	"testing"
)

// importMapScript matches the import map script of rendered HTML; the
// group is its text, as the browser hashes it for the CSP.
var importMapScript = regexp.MustCompile(`(?s)<script type="importmap"[^>]*>(.*?)</script>`)

// hashOfRendered recomputes the CSP hash from rendered HTML, independent of
// the code that renders it.
func hashOfRendered(t *testing.T, html template.HTML) string {
	t.Helper()

	match := importMapScript.FindStringSubmatch(string(html))
	if match == nil {
		t.Fatalf("no import map script in\n%s", html)
	}

	sum := sha256.Sum256([]byte(match[1]))

	return "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
}

// cspMap is an import map with every part that ends up in the script text:
// escaped characters, scopes and integrity.
func cspMap() *ImportMap {
	im := New().
		ShimPath("/assets/shim.js").
		WithImport("app", "/app.js?a=1&b=<2>", "").
		WithImport("lib", "/lib.js", "sha384-abc").
		ModulePreload("app", "lib")

	im.Structure.Scopes = map[string]map[string]string{
		"/admin/": {"app": "/admin/app.js"},
	}

	return im
}

// TestScriptHashMatchesRenderedText checks that ScriptHash is the hash of
// the text the same options render, for every combination of options.
func TestScriptHashMatchesRenderedText(t *testing.T) {
	cases := map[string][]RenderOption{
		"plain":            nil,
		"integrity":        {WithIntegrity()},
		"stable integrity": {WithStableIntegrity()},
		"nonce":            {WithNonce("abc")},
		"all":              {WithIntegrity(), WithNonce("abc")},
	}

	for name, opts := range cases {
		t.Run(name, func(t *testing.T) {
			im := cspMap()

			html, err := im.RenderWith(opts...)
			if err != nil {
				t.Fatal(err)
			}

			want := hashOfRendered(t, html)

			if got := im.ScriptHash(opts...); got != want {
				t.Errorf("ScriptHash = %s, rendered text hashes to %s", got, want)
			}

			withHTML, withHash, err := im.RenderWithHash(opts...)
			if err != nil {
				t.Fatal(err)
			}

			if withHTML != html {
				t.Errorf("RenderWithHash html =\n%s\nwant\n%s", withHTML, html)
			}

			if withHash != want {
				t.Errorf("RenderWithHash hash = %s, want %s", withHash, want)
			}
		})
	}
}

// TestScriptHashOfKnownText checks the format of the hash against a text
// written out by hand.
func TestScriptHashOfKnownText(t *testing.T) {
	im := New().WithImport("app", "/app.js", "")

	text := "\n{\n  \"imports\": {\n    \"app\": \"/app.js\"\n  }\n}\n"
	sum := sha256.Sum256([]byte(text))
	want := "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"

	if got := im.ScriptHash(); got != want {
		t.Fatalf("ScriptHash = %s, want %s", got, want)
	}

	html, err := im.Render()
	if err != nil {
		t.Fatal(err)
	}

	if string(html) != `<script type="importmap">`+text+`</script>` {
		t.Fatalf("render =\n%q", html)
	}
}

// TestScriptHashFollowsTheText checks which options change the hash: the
// integrity section does, the nonce, outside the text, does not; and that
// the render options of the map count, as for Render.
func TestScriptHashFollowsTheText(t *testing.T) {
	im := cspMap()
	plain := im.ScriptHash()

	if plain == "" {
		t.Fatal("no hash")
	}

	if got := im.ScriptHash(WithNonce("n")); got != plain {
		t.Errorf("the nonce changed the hash: %s, want %s", got, plain)
	}

	withIntegrity := im.ScriptHash(WithIntegrity())
	if withIntegrity == plain {
		t.Error("the integrity section did not change the hash")
	}

	im.WithRenderOptions(WithIntegrity())

	if got := im.ScriptHash(); got != withIntegrity {
		t.Errorf("ScriptHash ignores WithRenderOptions: %s, want %s", got, withIntegrity)
	}

	im.WithImport("other", "/other.js", "")

	if got := im.ScriptHash(); got == withIntegrity {
		t.Error("a new import did not change the hash")
	}
}

// TestScriptHashWithoutImportMap checks that a map that renders no import
// map script has no hash.
func TestScriptHashWithoutImportMap(t *testing.T) {
	im := New()
	im.Structure.Styles["site"] = "/site.css"

	if got := im.ScriptHash(); got != "" {
		t.Errorf("ScriptHash = %s, want empty", got)
	}

	html, hash, err := im.RenderWithHash()
	if err != nil {
		t.Fatal(err)
	}

	if hash != "" || strings.Contains(string(html), "importmap") {
		t.Errorf("RenderWithHash = %s, %q", html, hash)
	}
}

// TestWithNonce checks that the nonce is on every element the policy
// treats as a script, escaped, and on nothing else.
func TestWithNonce(t *testing.T) {
	im := cspMap()
	im.Structure.Styles["site"] = "/site.css"

	html, err := im.RenderWith(WithNonce(`a"b`), WithIntegrity())
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		`<link rel="stylesheet" href="/site.css"/>`,
		`<script async src="/assets/shim.js" nonce="a&#34;b"></script>`,
		`<script type="importmap" nonce="a&#34;b">` + "\n{",
		`<link rel="modulepreload" href="/app.js?a=1&amp;b=&lt;2&gt;" nonce="a&#34;b"/>`,
		`<link rel="modulepreload" href="/lib.js" integrity="sha384-abc" nonce="a&#34;b"/>`,
	} {
		if !strings.Contains(string(html), want) {
			t.Errorf("missing %s in\n%s", want, html)
		}
	}

	if got := strings.Count(string(html), "nonce="); got != 4 {
		t.Errorf("%d nonce attributes, want 4:\n%s", got, html)
	}

	empty, err := im.RenderWith(WithNonce(""))
	if err != nil {
		t.Fatal(err)
	}

	plain, err := im.Render()
	if err != nil {
		t.Fatal(err)
	}

	if empty != plain {
		t.Errorf("an empty nonce changed the render:\n%s\nwant\n%s", empty, plain)
	}
}

// TestImportmapNonceFunc checks the template function, and that the page
// it renders keeps the hash of {{importmap}}.
func TestImportmapNonceFunc(t *testing.T) {
	im := New().WithImport("app", "/app.js", "")
	tmpl := template.Must(template.New("head").Funcs(im.FuncMap()).Parse(`{{importmap_nonce .}}`))

	var out bytes.Buffer
	if err := tmpl.Execute(&out, "r4nd0m"); err != nil {
		t.Fatal(err)
	}

	if !strings.HasPrefix(out.String(), `<script type="importmap" nonce="r4nd0m">`) {
		t.Fatalf("template = %s", out.String())
	}

	if got := hashOfRendered(t, template.HTML(out.String())); got != im.ScriptHash() {
		t.Errorf("hash of the nonce render = %s, ScriptHash = %s", got, im.ScriptHash())
	}
}

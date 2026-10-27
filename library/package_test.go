package library

import (
	"strings"
	"testing"
)

func TestIncludesGetAndCheck(t *testing.T) {
	includes := Includes{{File: "dist/[a.js"}, {File: "dist/**.js", As: "app"}}

	// The invalid pattern matches nothing; the next one still does.
	if got := includes.Get("dist/app.js"); got == nil || got.As != "app" {
		t.Fatalf("Get = %+v, want the dist/**.js include", got)
	}

	err := includes.Check()
	if err == nil || !strings.Contains(err.Error(), `"dist/[a.js"`) {
		t.Fatalf("Check = %v, want an error naming the invalid pattern", err)
	}

	if err := (Includes{{File: "dist/**.js"}, {Raw: "https://cdn.example/x.js"}}).Check(); err != nil {
		t.Fatalf("Check of valid patterns = %v", err)
	}
}

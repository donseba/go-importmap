package raw

import (
	"encoding/json"
	"testing"
)

func TestNew(t *testing.T) {
	cdn := New("https://unpkg.com/browse/htmx.org@1.9.10/dist/htmx.min.js")

	f, v, err := cdn.FetchPackageFiles(t.Context(), "htmx.org", "1.9.10")
	if err != nil {
		t.Error(err)
		return
	}

	if v != "1.9.10" {
		t.Error("version mismatch")
	}

	if len(f) == 0 {
		t.Error("no files found")
	}

	out, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		t.Error(err)
		return
	}

	t.Log(string(out))
}

package jsdelivr

import (
	"encoding/json"
	"testing"
)

func TestNew(t *testing.T) {
	cdn := New()

	f, v, err := cdn.FetchPackageFiles(t.Context(), "bootstrap", "5.3.3")
	if err != nil {
		t.Error(err)
		return
	}

	if v != "5.3.3" {
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

func TestIncludeMinified(t *testing.T) {
	cdn := New()

	f, _, err := cdn.FetchPackageFiles(t.Context(), "@hotwired/turbo", "8.0.13")

	if err != nil {
		t.Error(err)
		return
	}

	if len(f) != 4 {
		t.Error("files count mismatch")
	}

	t.Log(f)
}

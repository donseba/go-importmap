package jsdelivr

import (
	"context"
	"encoding/json"
	"testing"
)

func TestNew(t *testing.T) {
	cdn := New()
	ctx := context.Background()

	f, v, err := cdn.FetchPackageFiles(ctx, "bootstrap", "5.3.3")
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

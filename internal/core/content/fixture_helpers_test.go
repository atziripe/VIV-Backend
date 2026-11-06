package content_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// writeJSONFixture marshals v and writes it to dir/filename — shared by
// session_library_test.go and exercise_library_test.go to build small,
// controlled LoadSessionLibrary/LoadExerciseLibrary fixtures without
// depending on the real dummy content under internal/content/training_v2/.
func writeJSONFixture(t *testing.T, dir, filename string, v any) {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshaling fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, filename), data, 0o644); err != nil {
		t.Fatalf("writing fixture %s: %v", filename, err)
	}
}

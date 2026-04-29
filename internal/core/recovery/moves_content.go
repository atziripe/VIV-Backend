package recovery

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ============================================================================
// PRE-GENERATED MOVE CONTENT
// ============================================================================

// MoveContent is the pre-generated language for a specific move in context.
type MoveContent struct {
	MoveID string `json:"move_id"`
	Title  string `json:"title"`  // max 8 words
	Timing string `json:"timing"` // max 6 words
	Detail string `json:"detail"` // max 35 words
	Impact string `json:"impact"` // from library: Highest, High, Medium
}

// MovesContentEntry is one pre-generated combination.
type MovesContentEntry struct {
	Phase               string        `json:"phase"`
	SessionType         string        `json:"session_type"`
	ConsecutiveRestDays int           `json:"consecutive_rest_days"`
	Moves               []MoveContent `json:"moves"`
}

// MovesContentLibrary holds all pre-generated move content.
type MovesContentLibrary struct {
	entries map[string][]MoveContent // key: "phase:session_type:rest_days"
}

// LoadMovesContentLibrary reads pre-generated move content from JSON files.
func LoadMovesContentLibrary(dir string) (*MovesContentLibrary, error) {
	lib := &MovesContentLibrary{
		entries: make(map[string][]MoveContent),
	}

	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".json" {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading %s: %w", path, err)
		}

		var entries []MovesContentEntry
		if err := json.Unmarshal(data, &entries); err != nil {
			// Try single entry
			var single MovesContentEntry
			if err := json.Unmarshal(data, &single); err != nil {
				return fmt.Errorf("parsing %s: %w", path, err)
			}
			entries = []MovesContentEntry{single}
		}

		for _, e := range entries {
			key := fmt.Sprintf("%s:%s:%d", e.Phase, e.SessionType, e.ConsecutiveRestDays)
			lib.entries[key] = e.Moves
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("loading moves content from %s: %w", dir, err)
	}

	return lib, nil
}

// GetMoves returns pre-generated move content for the given combination.
// Falls back to rest_days=1 if specific count not found.
func (l *MovesContentLibrary) GetMoves(phase string, sessionType string, restDays int) []MoveContent {
	key := fmt.Sprintf("%s:%s:%d", phase, sessionType, restDays)
	if moves, ok := l.entries[key]; ok {
		return moves
	}
	// Fallback to day 1
	key = fmt.Sprintf("%s:%s:1", phase, sessionType)
	if moves, ok := l.entries[key]; ok {
		return moves
	}
	return nil
}

// EntryCount returns total combinations loaded.
func (l *MovesContentLibrary) EntryCount() int {
	return len(l.entries)
}

package recovery

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"viv/internal/core/domain"
)

// ============================================================================
// BANNER LIBRARY — pre-generated body copy variants
// ============================================================================

// BannerLibrary holds pre-generated body copy indexed by
// phase + session_type + consecutive_rest_days (for R variants)
// phase + consecutive_rest_days (for N variants)
type BannerLibrary struct {
	recovery map[string]string // key: "phase:modality:rest_days" → body copy
	neutral  map[string]string // key: "phase:rest_days" → body copy
}

// BannerLibraryEntry is the JSON structure of each pre-generated variant.
type BannerLibraryEntry struct {
	Phase               string `json:"phase"`
	SessionType         string `json:"session_type,omitempty"` // empty for neutral variants
	ConsecutiveRestDays int    `json:"consecutive_rest_days"`
	Variant             string `json:"variant"` // "R" or "N"
	Body                string `json:"body"`
}

// LoadBannerLibrary reads all banner variants from JSON files.
func LoadBannerLibrary(dir string) (*BannerLibrary, error) {
	lib := &BannerLibrary{
		recovery: make(map[string]string),
		neutral:  make(map[string]string),
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

		// Try parsing as array first (batch file), then single entry
		var entries []BannerLibraryEntry
		if err := json.Unmarshal(data, &entries); err != nil {
			var single BannerLibraryEntry
			if err := json.Unmarshal(data, &single); err != nil {
				return fmt.Errorf("parsing %s: %w", path, err)
			}
			entries = []BannerLibraryEntry{single}
		}

		for _, e := range entries {
			switch e.Variant {
			case "R":
				key := fmt.Sprintf("%s:%s:%d", e.Phase, e.SessionType, e.ConsecutiveRestDays)
				lib.recovery[key] = e.Body
			case "N":
				key := fmt.Sprintf("%s:%d", e.Phase, e.ConsecutiveRestDays)
				lib.neutral[key] = e.Body
			}
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("loading banner library from %s: %w", dir, err)
	}

	return lib, nil
}

// GetRecoveryBody returns body copy for a rest-after-session day (Variant R).
// Falls back to rest_days=1 if specific day count not found.
func (l *BannerLibrary) GetRecoveryBody(phase domain.CyclePhase, modality domain.Modality, restDays int) string {
	key := fmt.Sprintf("%s:%s:%d", phase, modality, restDays)
	if body, ok := l.recovery[key]; ok {
		return body
	}
	// Fallback to day 1
	key = fmt.Sprintf("%s:%s:1", phase, modality)
	if body, ok := l.recovery[key]; ok {
		return body
	}
	return ""
}

// GetNeutralBody returns body copy for consecutive rest days (Variant N).
// Falls back to rest_days=2 if specific day count not found.
func (l *BannerLibrary) GetNeutralBody(phase domain.CyclePhase, restDays int) string {
	key := fmt.Sprintf("%s:%d", phase, restDays)
	if body, ok := l.neutral[key]; ok {
		return body
	}
	// Fallback to day 2
	key = fmt.Sprintf("%s:2", phase)
	if body, ok := l.neutral[key]; ok {
		return body
	}
	return ""
}

// EntryCount returns total entries loaded.
func (l *BannerLibrary) EntryCount() int {
	return len(l.recovery) + len(l.neutral)
}

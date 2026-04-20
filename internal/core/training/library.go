package training

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"

	"viv/internal/core/domain"
)

// ============================================================================
// LIBRARY — carga y almacena las sesiones de la librería
// ============================================================================

// Library contiene todas las sesiones cargadas desde los archivos JSON.
// Se construye una vez al boot del servicio y se reutiliza en cada request.
// Es inmutable después de la carga — safe para uso concurrente.
type Library struct {
	sessions []domain.LibrarySession
}

// LoadLibrary lee todos los archivos JSON del directorio dado (recursivo)
// y los carga en memoria. Devuelve error si no encuentra archivos o si
// alguno no se puede parsear.
//
// Uso típico al boot:
//
//	lib, err := training.LoadLibrary("internal/content/training")
//	if err != nil {
//	    log.Fatalf("failed to load training library: %v", err)
//	}
func LoadLibrary(dir string) (*Library, error) {
	var sessions []domain.LibrarySession

	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(d.Name()), ".json") {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading %s: %w", path, err)
		}

		var s domain.LibrarySession
		if err := json.Unmarshal(data, &s); err != nil {
			return fmt.Errorf("parsing %s: %w", path, err)
		}

		if s.ID == "" {
			return fmt.Errorf("session in %s has empty id", path)
		}

		sessions = append(sessions, s)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("loading training library from %s: %w", dir, err)
	}

	if len(sessions) == 0 {
		return nil, fmt.Errorf("no training sessions found in %s", dir)
	}

	return &Library{sessions: sessions}, nil
}

func (l *Library) SessionCount() int {
	return len(l.sessions)
}

// SELECTOR

type SelectionCriteria struct {
	// Hard filters — should match
	Session domain.TrainingSession // modality + muscle_group + intensity
	Phase   domain.CyclePhase

	// Soft filters — check-in preferences (optional for MVP)
	RecoveryCapacity string // "low" | "moderate" | "high" | "" (ignore)
	LifeBandwidth    string // "low" | "moderate" | "high" | "" (ignore)
	BuildReadiness   string // "pull_back" | "maintain" | "push_forward" | "" (ignore)
}

// SelectSession busca la mejor sesión de la librería que matchee los criterios.
// Devuelve la sesión seleccionada y true si encontró match, o zero value y false
// si no hay ninguna sesión que cumpla los filtros hard.
//
// Algoritmo:
//  1. Filtra por hard criteria (modality, muscle_group, intensity, phase).
//  2. Si RecoveryCapacity == "low", excluye recovery_demand == "high".
//  3. Ordena candidatos por preferencias soft (time_commitment, progression_role).
//  4. Si hay múltiples candidatos equivalentes, elige uno al azar.
func (l *Library) SelectSession(criteria SelectionCriteria) (domain.LibrarySession, bool) {
	// Paso 1: hard filters
	candidates := l.filterHard(criteria)
	if len(candidates) == 0 {
		return domain.LibrarySession{}, false
	}

	// exclude high recovery demand when capacity is low (Optional)
	if criteria.RecoveryCapacity == "low" {
		filtered := filterByRecoveryDemand(candidates)
		if len(filtered) > 0 {
			candidates = filtered
		}
	}

	// soft preferences
	candidates = rankByPreferences(candidates, criteria)

	//pick from top candidates
	return candidates[0], true
}

// filterHard devuelve solo las sesiones que matchean los filtros obligatorios.
func (l *Library) filterHard(criteria SelectionCriteria) []domain.LibrarySession {
	var out []domain.LibrarySession
	for _, s := range l.sessions {
		if !s.MatchesSession(criteria.Session) {
			continue
		}
		if !s.SuitableForPhase(criteria.Phase) {
			continue
		}
		out = append(out, s)
	}
	return out
}

// filterByRecoveryDemand excluye sesiones con recovery_demand == "high".
func filterByRecoveryDemand(sessions []domain.LibrarySession) []domain.LibrarySession {
	var out []domain.LibrarySession
	for _, s := range sessions {
		if s.RecoveryDemand != "high" {
			out = append(out, s)
		}
	}
	return out
}

// rankByPreferences ordena candidatos según las preferencias soft del check-in.
// No elimina candidatos — solo reordena. El mejor match queda primero.
//
// Scoring simple: cada preferencia que matchea suma 1 punto.
// En caso de empate, se randomiza para variedad.
func rankByPreferences(candidates []domain.LibrarySession, criteria SelectionCriteria) []domain.LibrarySession {
	if len(candidates) <= 1 {
		return candidates
	}

	type scored struct {
		session domain.LibrarySession
		score   int
	}

	scored_list := make([]scored, len(candidates))
	for i, s := range candidates {
		score := 0

		// Prefer low time_commitment when bandwidth is low
		if criteria.LifeBandwidth == "low" && s.TimeCommitment == "low" {
			score++
		}

		// Prefer matching progression_role to build readiness
		switch criteria.BuildReadiness {
		case "pull_back":
			if s.ProgressionRole == "recover" || s.ProgressionRole == "maintain" {
				score++
			}
		case "push_forward":
			if s.ProgressionRole == "build" {
				score++
			}
		case "maintain":
			if s.ProgressionRole == "maintain" {
				score++
			}
		}

		scored_list[i] = scored{session: s, score: score}
	}

	// Sort by score descending, randomize ties
	for i := 0; i < len(scored_list)-1; i++ {
		for j := i + 1; j < len(scored_list); j++ {
			if scored_list[j].score > scored_list[i].score {
				scored_list[i], scored_list[j] = scored_list[j], scored_list[i]
			} else if scored_list[j].score == scored_list[i].score && rand.Intn(2) == 0 {
				scored_list[i], scored_list[j] = scored_list[j], scored_list[i]
			}
		}
	}

	result := make([]domain.LibrarySession, len(scored_list))
	for i, s := range scored_list {
		result[i] = s.session
	}
	return result
}

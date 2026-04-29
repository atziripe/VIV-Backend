package recovery

// ============================================================================
// MOVE LIBRARY — 22 hardcoded moves, 5 categories
// ============================================================================

// MoveDefinition is a single recovery move in the library.
type MoveDefinition struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Category string `json:"category"` // sleep, nervous_system, joint, fuel, hydration
	Impact   string `json:"impact"`   // Highest, High, Medium
}

var moveLibrary = map[string]MoveDefinition{
	// Sleep
	"S1": {ID: "S1", Name: "Sleep duration protocol", Category: "sleep", Impact: "High"},
	"S2": {ID: "S2", Name: "Sleep hygiene — late luteal variant", Category: "sleep", Impact: "Highest"},
	"S3": {ID: "S3", Name: "Pre-sleep temperature protocol", Category: "sleep", Impact: "High"},
	"S4": {ID: "S4", Name: "Screen cutoff protocol", Category: "sleep", Impact: "Medium"},

	// Nervous system
	"N1": {ID: "N1", Name: "4-7-8 breathing reset", Category: "nervous_system", Impact: "High"},
	"N2": {ID: "N2", Name: "10 min walk — no input", Category: "nervous_system", Impact: "Medium"},
	"N3": {ID: "N3", Name: "Diaphragmatic breathing — post training", Category: "nervous_system", Impact: "High"},
	"N4": {ID: "N4", Name: "Legs up the wall — 5 min", Category: "nervous_system", Impact: "High"},
	"N5": {ID: "N5", Name: "Cold face immersion — vagus nerve", Category: "nervous_system", Impact: "Medium"},

	// Joint
	"J1": {ID: "J1", Name: "Joint mobility — hips, knees, ankles", Category: "joint", Impact: "Highest"},
	"J2": {ID: "J2", Name: "Foam rolling — lower body", Category: "joint", Impact: "High"},
	"J3": {ID: "J3", Name: "Gentle movement — stiffness prevention", Category: "joint", Impact: "Medium"},
	"J4": {ID: "J4", Name: "Thoracic spine mobility", Category: "joint", Impact: "High"},

	// Fuel
	"F1": {ID: "F1", Name: "Protein target — hit daily goal", Category: "fuel", Impact: "Medium"},
	"F2": {ID: "F2", Name: "Post-training protein window", Category: "fuel", Impact: "High"},
	"F3": {ID: "F3", Name: "Iron-rich meal", Category: "fuel", Impact: "Highest"},
	"F4": {ID: "F4", Name: "Glycogen replenishment — carbs + protein", Category: "fuel", Impact: "High"},

	// Hydration
	"H1": {ID: "H1", Name: "Scheduled hydration — sip don't chug", Category: "hydration", Impact: "Medium"},
	"H2": {ID: "H2", Name: "Hydration with electrolytes", Category: "hydration", Impact: "High"},
	"H3": {ID: "H3", Name: "Pre-training hydration protocol", Category: "hydration", Impact: "High"},
	"H4": {ID: "H4", Name: "Electrolyte protocol — menstrual", Category: "hydration", Impact: "Highest"},
}

// GetMove returns a move definition by ID.
func GetMove(id string) (MoveDefinition, bool) {
	m, ok := moveLibrary[id]
	return m, ok
}

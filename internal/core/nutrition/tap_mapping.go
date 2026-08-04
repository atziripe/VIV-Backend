package nutrition

import "strings"

// ============================================================================
// TAG MAPPING — onboarding raw UI labels -> ingredient library vocabulary.
// ============================================================================
// domain.User/usecase.MealGenerationInput carry onboarding answers as raw
// strings (see user_parser.go for the same pattern in training). These
// translate them into the contains/digestion_flags/cuisine/eating_style_tags
// vocabulary the ingredient library actually filters on.

var restrictionToContainsTag = map[string]string{
	"no meat":           "meat",
	"no fish":           "fish",
	"no eggs":           "egg",
	"no dairy":          "dairy",
	"no gluten":         "gluten",
	"no caffeine":       "caffeine",
	"nuts allergy":      "nuts",
	"shellfish allergy": "shellfish",
}

// MapRestrictionsToContainsTags converts raw onboarding restriction labels
// ("No meat", "Nuts allergy", ...) into the ingredient library's contains
// vocabulary. Unrecognized labels (including "None") are dropped rather than
// erroring — an unmapped restriction must never silently become a hard
// filter that excludes everything.
func MapRestrictionsToContainsTags(restrictions []string) []string {
	var out []string
	for _, r := range restrictions {
		if tag, ok := restrictionToContainsTag[strings.ToLower(strings.TrimSpace(r))]; ok {
			out = append(out, tag)
		}
	}
	return out
}

var digestiveToFlag = map[string]string{
	"bloating":               "bloating",
	"reflux":                 "reflux",
	"sensitive stomach":      "sensitive_stomach",
	"constipation":           "constipation",
	"ibs – like sensitivity": "ibs_like_sensitivity",
	"ibs - like sensitivity": "ibs_like_sensitivity", // hyphen variant
	"ibs like sensitivity":   "ibs_like_sensitivity",
}

// MapDigestiveConditionsToFlags converts raw onboarding digestion labels
// into the ingredient library's digestion_flags vocabulary. "None"/"Rarely"
// have no entry and are dropped, same as an unmapped restriction.
func MapDigestiveConditionsToFlags(conditions []string) []string {
	var out []string
	for _, c := range conditions {
		if flag, ok := digestiveToFlag[strings.ToLower(strings.TrimSpace(c))]; ok {
			out = append(out, flag)
		}
	}
	return out
}

// ProteinProfile mirrors the 3 answers to "Your usual protein resources".
type ProteinProfile string

const (
	ProfilePlantBased  ProteinProfile = "plant_based"
	ProfileMixed       ProteinProfile = "mixed"
	ProfileAnimalBased ProteinProfile = "animal_based"
)

// MapProteinPreference classifies the raw onboarding answer. Matches by
// substring rather than exact string so a small copy change upstream
// ("Mostly plant-based" vs "Plant-based") doesn't silently stop matching.
// "mixed" is checked first and wins outright — the UI's mixed option reads
// "Mixed plant + animal", which contains "plant" and was silently getting
// classified as strictly plant-based, filtering every animal-sourced
// protein out for anyone who picked "mixed".
func MapProteinPreference(pref string) ProteinProfile {
	p := strings.ToLower(pref)
	switch {
	case strings.Contains(p, "mixed"):
		return ProfileMixed
	case strings.Contains(p, "plant"):
		return ProfilePlantBased
	case strings.Contains(p, "animal"):
		return ProfileAnimalBased
	default:
		return ProfileMixed
	}
}

var eatingStyleLabelToTag = map[string]string{
	"simple & quick": "simple_quick",
	"home-cooked":    "home_cooked",
	"eat out often":  "eat_out_friendly",
	"meal prep":      "meal_prep_friendly",
}

var cuisineLabels = map[string]string{
	"mediterranean":  "mediterranean",
	"asian":          "asian",
	"latin":          "latin",
	"american":       "american",
	"middle eastern": "middle_eastern",
}

// ParseEatingStyleAndCuisine splits the combined onboarding field into the
// library's separate vocabularies. Style and cuisine arrive as one
// comma-joined string in domain.User.EatingStyle (there is no separate
// Cuisine field — see meal_generator.go's buildMealUserPrompt for the same
// merge on the LLM-based path).
func ParseEatingStyleAndCuisine(eatingStyle string) (styleTags []string, cuisine string) {
	for _, part := range strings.Split(eatingStyle, ",") {
		key := strings.ToLower(strings.TrimSpace(part))
		if key == "" {
			continue
		}
		if tag, ok := eatingStyleLabelToTag[key]; ok {
			styleTags = append(styleTags, tag)
			continue
		}
		if c, ok := cuisineLabels[key]; ok {
			cuisine = c
		}
	}
	return styleTags, cuisine
}

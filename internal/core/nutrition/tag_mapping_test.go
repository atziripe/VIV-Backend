package nutrition

import "testing"

func TestMapProteinPreference(t *testing.T) {
	cases := []struct {
		in   string
		want ProteinProfile
	}{
		{"Mixed plant + animal", ProfileMixed}, // the actual onboarding UI label — regression for the "mixed" being misread as "plant" bug
		{"Mixed", ProfileMixed},
		{"Mostly plant-based", ProfilePlantBased},
		{"Plant based", ProfilePlantBased},
		{"Mostly animal-based", ProfileAnimalBased},
		{"", ProfileMixed},
	}
	for _, c := range cases {
		if got := MapProteinPreference(c.in); got != c.want {
			t.Errorf("MapProteinPreference(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

package nutrition

import (
	"testing"

	"viv/internal/core/domain"
)

func TestCompositionKey_SameCompositionSameKey(t *testing.T) {
	a := []domain.MealIngredient{
		{Name: "Chicken breast", AmountG: 150.2},
		{Name: "White rice, cooked", AmountG: 200},
	}
	b := []domain.MealIngredient{
		// same ingredients, different order, trivially different float
		{Name: "White rice, cooked", AmountG: 198},
		{Name: "Chicken breast", AmountG: 152},
	}

	ka := CompositionKey("Mediterranean lunch bowl", a)
	kb := CompositionKey("Mediterranean lunch bowl", b)

	if ka != kb {
		t.Errorf("expected matching keys for the same composition (order-independent, 5g-rounded), got %q vs %q", ka, kb)
	}
}

func TestCompositionKey_DifferentAmountsDifferentKey(t *testing.T) {
	base := []domain.MealIngredient{{Name: "Chicken breast", AmountG: 150}}
	other := []domain.MealIngredient{{Name: "Chicken breast", AmountG: 220}}

	k1 := CompositionKey("tpl", base)
	k2 := CompositionKey("tpl", other)

	if k1 == k2 {
		t.Errorf("expected different keys for meaningfully different amounts, both got %q", k1)
	}
}

func TestCompositionKey_DifferentTemplateDifferentKey(t *testing.T) {
	ings := []domain.MealIngredient{{Name: "Chicken breast", AmountG: 150}}

	k1 := CompositionKey("Mediterranean lunch bowl", ings)
	k2 := CompositionKey("Latin lunch bowl", ings)

	if k1 == k2 {
		t.Errorf("expected different keys for different templates with the same ingredients, both got %q", k1)
	}
}

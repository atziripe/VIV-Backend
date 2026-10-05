-- New pipeline's single standalone nutrition plan — one row per user,
-- overwritten on each regeneration (no per-generation versioning, unlike
-- `plans`). plan_json holds the full domain.NutritionWeekPlan blob.
CREATE TABLE nutrition_plans (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    plan_json   JSONB NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_nutrition_plans_gin ON nutrition_plans USING GIN(plan_json);

-- VIV-108 real implementation: replaces the flat exercise_ids list (the
-- StubExerciseLibrary-era placeholder shape) with the Block-Config-driven
-- model — one exercise per movement-group slot, plus the block number
-- rotation is keyed off. active_exercises is a JSON object
-- ({"Squat": "lower_goblet_squat", "Hip hinge": "lower_romanian_deadlift", ...})
-- rather than a join table, since the repository never queries into it —
-- it's always read/written whole, exactly like exercise_ids was.
ALTER TABLE exercise_pins ADD COLUMN active_exercises JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE exercise_pins ADD COLUMN block_number INT NOT NULL DEFAULT 1;
ALTER TABLE exercise_pins DROP COLUMN exercise_ids;

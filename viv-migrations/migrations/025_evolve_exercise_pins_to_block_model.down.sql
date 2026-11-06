ALTER TABLE exercise_pins ADD COLUMN exercise_ids TEXT[] NOT NULL DEFAULT '{}';
ALTER TABLE exercise_pins DROP COLUMN active_exercises;
ALTER TABLE exercise_pins DROP COLUMN block_number;

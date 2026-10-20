ALTER TABLE plans
    DROP COLUMN IF EXISTS start_date,
    DROP COLUMN IF EXISTS end_date,
    DROP COLUMN IF EXISTS generated_from,
    DROP COLUMN IF EXISTS source_event_id;

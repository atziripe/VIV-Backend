-- Fix: cycle_duration was INTEGER but domain.User.CycleDuration is string ("28")
ALTER TABLE user_cycles ALTER COLUMN cycle_duration TYPE TEXT USING cycle_duration::TEXT;

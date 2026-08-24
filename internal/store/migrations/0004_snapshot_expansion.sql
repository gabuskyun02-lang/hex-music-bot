-- 0004_snapshot_expansion: restart-survival state flags for G5-C restore.
ALTER TABLE player_snapshots ADD COLUMN paused INTEGER NOT NULL DEFAULT 0;
ALTER TABLE player_snapshots ADD COLUMN shuffled INTEGER NOT NULL DEFAULT 0;
ALTER TABLE player_snapshots ADD COLUMN previous_identifier TEXT NOT NULL DEFAULT '';

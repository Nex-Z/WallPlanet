DROP INDEX IF EXISTS jobs_one_source_active;
CREATE UNIQUE INDEX jobs_one_source_active ON jobs(source_id) WHERE status IN ('queued','starting','running','importing','uncertain','retrying');
CREATE INDEX IF NOT EXISTS interactions_wallpaper ON interactions(wallpaper_id,kind,created_at);
CREATE INDEX IF NOT EXISTS downloads_rank ON downloads(created_at,wallpaper_id,user_id);
CREATE INDEX IF NOT EXISTS media_wallpaper ON media(wallpaper_id,position);
CREATE INDEX IF NOT EXISTS media_storage ON media(storage_key);
CREATE INDEX IF NOT EXISTS media_thumbnail ON media(thumbnail_key);
CREATE INDEX IF NOT EXISTS subscriptions_entity ON subscriptions(entity_id);

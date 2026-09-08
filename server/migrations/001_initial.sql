CREATE TABLE IF NOT EXISTS users (
 id text PRIMARY KEY, username text NOT NULL UNIQUE, password_hash text NOT NULL,
 role text NOT NULL DEFAULT 'user' CHECK(role IN ('user','admin')), disabled boolean NOT NULL DEFAULT false,
 profile jsonb NOT NULL DEFAULT '{}', created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS sessions (
 token_hash text PRIMARY KEY, user_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 csrf text NOT NULL, expires_at timestamptz NOT NULL
);
CREATE INDEX IF NOT EXISTS sessions_expiry ON sessions(expires_at);
CREATE TABLE IF NOT EXISTS entities (
 id text PRIMARY KEY, kind text NOT NULL CHECK(kind IN ('author','topic','channel')),
 data jsonb NOT NULL DEFAULT '{}', created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS wallpapers (
 id text PRIMARY KEY, source_id text NOT NULL UNIQUE, author_id text REFERENCES entities(id),
 channel_id text NOT NULL REFERENCES entities(id), title text NOT NULL, description text NOT NULL DEFAULT '',
 tags text[] NOT NULL DEFAULT '{}', topic_ids text[] NOT NULL DEFAULT '{}',
 source_url text NOT NULL DEFAULT '', source_metrics jsonb NOT NULL DEFAULT '{}',
 published_at timestamptz NOT NULL DEFAULT now(), status text NOT NULL DEFAULT 'published' CHECK(status IN ('published','hidden')),
 featured boolean NOT NULL DEFAULT false, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS wallpapers_order ON wallpapers(published_at DESC,id DESC);
CREATE INDEX IF NOT EXISTS wallpapers_topics ON wallpapers USING gin(topic_ids);
CREATE INDEX IF NOT EXISTS wallpapers_tags ON wallpapers USING gin(tags);
CREATE TABLE IF NOT EXISTS media (
 id text PRIMARY KEY, wallpaper_id text NOT NULL REFERENCES wallpapers(id) ON DELETE CASCADE,
 source_key text NOT NULL, storage_key text NOT NULL, thumbnail_key text NOT NULL,
 width integer NOT NULL, height integer NOT NULL, bytes bigint NOT NULL, mime text NOT NULL,
 position integer NOT NULL, UNIQUE(wallpaper_id,source_key)
);
CREATE TABLE IF NOT EXISTS interactions (
 user_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 wallpaper_id text NOT NULL REFERENCES wallpapers(id) ON DELETE CASCADE,
 kind text NOT NULL CHECK(kind IN ('like','favorite')), created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(user_id,wallpaper_id,kind)
);
CREATE TABLE IF NOT EXISTS downloads (
 id text PRIMARY KEY, user_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 wallpaper_id text NOT NULL REFERENCES wallpapers(id) ON DELETE CASCADE,
 media_id text NOT NULL REFERENCES media(id), created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS downloads_user ON downloads(user_id,created_at DESC);
CREATE TABLE IF NOT EXISTS history (
 user_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 wallpaper_id text NOT NULL REFERENCES wallpapers(id) ON DELETE CASCADE,
 viewed_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY(user_id,wallpaper_id)
);
CREATE TABLE IF NOT EXISTS subscriptions (
 user_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 entity_id text NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
 created_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY(user_id,entity_id)
);
CREATE TABLE IF NOT EXISTS settings (key text PRIMARY KEY, value jsonb NOT NULL, updated_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE IF NOT EXISTS sources (
 id text PRIMARY KEY, name text NOT NULL, kind text NOT NULL CHECK(kind IN ('account','keyword')),
 query text NOT NULL, enabled boolean NOT NULL DEFAULT false,
 interval_hours integer NOT NULL DEFAULT 6 CHECK(interval_hours BETWEEN 1 AND 720),
 max_items integer NOT NULL DEFAULT 100 CHECK(max_items BETWEEN 50 AND 1000),
 min_short integer NOT NULL DEFAULT 720 CHECK(min_short>=1), min_long integer NOT NULL DEFAULT 1280 CHECK(min_long>=1),
 topic_ids text[] NOT NULL DEFAULT '{}', tags text[] NOT NULL DEFAULT '{}',
 watermark timestamptz, next_run timestamptz NOT NULL DEFAULT now(), created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS jobs (
 id text PRIMARY KEY, source_id text NOT NULL REFERENCES sources(id),
 status text NOT NULL DEFAULT 'queued', run_id text, dataset_id text, input jsonb,
 dataset_offset integer NOT NULL DEFAULT 0, imported integer NOT NULL DEFAULT 0, skipped integer NOT NULL DEFAULT 0,
 error text NOT NULL DEFAULT '', started_at timestamptz, finished_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS jobs_one_source_active ON jobs(source_id) WHERE status IN ('queued','starting','running','importing','uncertain');
CREATE TABLE IF NOT EXISTS job_items (
 id text PRIMARY KEY, job_id text NOT NULL REFERENCES jobs(id), source_key text NOT NULL,
 payload jsonb NOT NULL, error text NOT NULL, attempts integer NOT NULL DEFAULT 1,
 resolved boolean NOT NULL DEFAULT false, UNIQUE(job_id,source_key)
);
CREATE TABLE IF NOT EXISTS rank_snapshots (
 period text NOT NULL, wallpaper_id text NOT NULL REFERENCES wallpapers(id) ON DELETE CASCADE,
 score bigint NOT NULL, updated_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY(period,wallpaper_id)
);
INSERT INTO entities(id,kind,data) VALUES ('x','channel','{"name":"X","description":"来自 X 的视觉灵感与创作者","avatar":"","featured":true}') ON CONFLICT DO NOTHING;
INSERT INTO entities(id,kind,data) VALUES
 ('nature','topic','{"name":"自然风光","description":"山川湖海，自有答案","cover":"","featured":true}'),
 ('anime','topic','{"name":"动漫插画","description":"走进想象中的世界","cover":"","featured":true}'),
 ('city','topic','{"name":"城市建筑","description":"记录城市的每一束光","cover":"","featured":true}'),
 ('minimal','topic','{"name":"极简主义","description":"留一点空白给生活","cover":"","featured":true}'),
 ('tech','topic','{"name":"科技未来","description":"想象下一个世界","cover":"","featured":true}'),
 ('car','topic','{"name":"汽车机械","description":"速度与设计的美感","cover":"","featured":true}'),
 ('space','topic','{"name":"星空宇宙","description":"在星辰之间漫游","cover":"","featured":true}'),
 ('art','topic','{"name":"艺术创意","description":"让灵感自由生长","cover":"","featured":true}') ON CONFLICT DO NOTHING;

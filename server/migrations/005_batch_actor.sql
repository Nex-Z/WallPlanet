ALTER TABLE sources DROP CONSTRAINT sources_kind_check;
ALTER TABLE sources ADD CONSTRAINT sources_kind_check CHECK(kind IN ('account','accounts','keyword'));
ALTER TABLE sources DROP CONSTRAINT sources_max_items_check;
ALTER TABLE sources ADD CONSTRAINT sources_max_items_check CHECK(max_items BETWEEN 1 AND 1000);
ALTER TABLE sources ALTER COLUMN interval_hours SET DEFAULT 168;
-- Preserve the actor identity of existing runs, including unfinished imports.
ALTER TABLE jobs ADD COLUMN actor text NOT NULL DEFAULT 'xtdata/twitter-x-scraper';
ALTER TABLE jobs ALTER COLUMN actor SET DEFAULT 'xquik/x-tweet-scraper';
UPDATE sources SET enabled=false WHERE kind='account';
UPDATE jobs SET status='failed',error='Actor 已切换，请使用作者组重新提交采集',finished_at=now() WHERE status='queued';

ALTER TABLE jobs ADD COLUMN upstream_error text NOT NULL DEFAULT '';

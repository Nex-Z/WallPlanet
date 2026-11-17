ALTER TABLE wallpapers ADD COLUMN model_names text[] NOT NULL DEFAULT '{}';
CREATE INDEX wallpapers_xrw_models ON wallpapers USING gin(model_names)
 WHERE channel_id='xrw-album' AND status='published';

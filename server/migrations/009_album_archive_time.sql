ALTER TABLE wallpapers ADD COLUMN album_archived_at timestamptz;
CREATE INDEX wallpapers_archive_order ON wallpapers ((COALESCE(album_archived_at,created_at)) DESC,id)
 WHERE status='published';

ALTER TABLE wallpapers ADD COLUMN album_group text NOT NULL DEFAULT '',
 ADD COLUMN album_title text NOT NULL DEFAULT '';
CREATE INDEX wallpapers_xrw_album_group ON wallpapers(album_group,id)
 WHERE channel_id='xrw-album' AND status='published' AND album_group<>'';

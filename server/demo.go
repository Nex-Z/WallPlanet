package main

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"path/filepath"
	"time"
)

func seedDemo(ctx context.Context, db *pgxpool.Pool) error {
	storage := &LocalStorage{Root: filepath.Join(env("DATA_DIR", "../data"), "demo")}
	authors := []struct{ id, name, description, avatar string }{{"demo-nature", "鹿与森林", "在山川之间，寻找生活的回声。", "lake"}, {"demo-city", "城市漫游", "收集日落之后的城市。", "city"}, {"demo-art", "Chill Zone", "让想象，在日常中生长。", "portrait"}, {"demo-space", "星际旅人", "去看更大的世界。", "space"}}
	for _, au := range authors {
		_, e := db.Exec(ctx, "INSERT INTO entities(id,kind,data) VALUES($1,'author',$2) ON CONFLICT(id) DO UPDATE SET data=excluded.data", au.id, encode(M{"name": au.name, "description": au.description, "avatar": "/assets/" + au.avatar + ".jpg", "featured": true}))
		if e != nil {
			return e
		}
	}
	items := []struct{ key, title, desc, topic, author string }{{"lake", "日照金山", "晨光洒在雪山之巅，世界如此宁静美好。", "nature", "demo-nature"}, {"portrait", "风与自由", "风经过的时候，把心事留给蓝天。", "anime", "demo-art"}, {"city", "霞光之城", "城市的灯火，从不辜负夜晚。", "city", "demo-city"}, {"ocean", "蓝色梦境", "当深蓝之下，你我与星空。", "nature", "demo-nature"}, {"cat", "午后片刻", "慢一点，和阳光一起发呆。", "art", "demo-art"}, {"forest", "森林私语", "藏在树影里的治愈。", "nature", "demo-nature"}, {"car", "赛道之魂", "速度即是浪漫。", "car", "demo-city"}, {"abstract", "流动的蓝", "每一条曲线，都有自己的方向。", "minimal", "demo-art"}, {"flowers", "秒速时刻", "樱花散落街角的那一秒。", "art", "demo-art"}, {"space", "宇宙漫游", "去看更大的世界。", "space", "demo-space"}, {"mountain", "雪峰黎明", "群山回应了第一束光。", "nature", "demo-nature"}, {"aurora", "极光来信", "来自遥远北方的一封信。", "space", "demo-space"}, {"architecture", "极简心境", "少即是多。", "minimal", "demo-city"}, {"clouds", "云端漫步", "在天空的柔软处停留。", "nature", "demo-nature"}}
	for i, v := range items {
		wid := "demo-" + v.key
		_, e := db.Exec(ctx, "INSERT INTO wallpapers(id,source_id,author_id,channel_id,title,description,tags,topic_ids,published_at,featured) VALUES($1,$2,$3,'x',$4,$5,$6,$7,$8,true) ON CONFLICT(id) DO UPDATE SET title=excluded.title,description=excluded.description", wid, "demo:"+v.key, v.author, v.title, v.desc, []string{"精选", v.title}, []string{v.topic}, time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC).Add(-time.Duration(i)*time.Hour))
		if e != nil {
			return e
		}
		keys := []string{v.key}
		if i == 0 {
			keys = []string{"lake", "mountain", "forest", "aurora"}
		}
		for pos, key := range keys {
			b, e := os.ReadFile(filepath.Join("../web/public/assets", key+".jpg"))
			if e != nil {
				return fmt.Errorf("missing demo image %s", key)
			}
			s, e := storage.Save(ctx, b)
			if e != nil {
				return e
			}
			_, e = db.Exec(ctx, "INSERT INTO media(id,wallpaper_id,source_key,storage_key,thumbnail_key,width,height,bytes,mime,position) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT(wallpaper_id,source_key) DO UPDATE SET storage_key=excluded.storage_key,thumbnail_key=excluded.thumbnail_key,width=excluded.width,height=excluded.height,bytes=excluded.bytes,mime=excluded.mime,position=excluded.position", wid+"-"+key, wid, key, s.Key, s.Thumbnail, s.Width, s.Height, s.Bytes, s.Mime, pos)
			if e != nil {
				return e
			}
		}
	}
	for _, v := range items {
		_, e := db.Exec(ctx, "UPDATE entities SET data=jsonb_set(data,'{cover}',to_jsonb($2::text)) WHERE id=$1", v.topic, "/assets/"+v.key+".jpg")
		if e != nil {
			return e
		}
	}
	_, e := db.Exec(ctx, "INSERT INTO settings(key,value) VALUES('homepage',$1) ON CONFLICT(key) DO UPDATE SET value=excluded.value", encode(M{"banners": []M{{"title": "让每一次打开屏幕\n都是一次新的出发", "subtitle": "精选全球优质壁纸 · 每日更新", "image": "/assets/lake.jpg", "href": "/wallpaper/demo-lake"}, {"title": "走进山川湖海\n发现世界的另一面", "subtitle": "把喜欢的风景，留在眼前", "image": "/assets/mountain.jpg", "href": "/discover?topic=nature"}, {"title": "在星辰之间\n寻找属于你的光", "subtitle": "探索星空宇宙专题", "image": "/assets/aurora.jpg", "href": "/discover?topic=space"}}}))
	return e
}

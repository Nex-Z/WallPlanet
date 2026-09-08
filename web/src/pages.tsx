import { useEffect, useRef, useState } from "react";
import { Link, useParams, useSearchParams } from "react-router-dom";
import {
  useInfiniteQuery,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { AnimatePresence, motion, useReducedMotion } from "motion/react";
import {
  ArrowRight,
  ArrowDownToLine,
  ChevronLeft,
  ChevronRight,
  Heart,
  Share2,
  MessageCircle,
  MoreHorizontal,
  Expand,
  X,
  Crown,
  RefreshCw,
  Image,
  CalendarDays,
  Globe,
  HardDrive,
  Check,
  Search,
  Compass,
} from "lucide-react";
import {
  api,
  downloadFile,
  number,
  params,
  type Entity,
  type Page,
  type Wallpaper,
} from "./api";
import { useApp } from "./context";
import {
  Avatar,
  Empty,
  ErrorState,
  Filters,
  Loading,
  SectionTitle,
  Sidebars,
  SmartImage,
  Subscribe,
  WallpaperGrid,
} from "./components";
type Banner = { title: string; subtitle: string; image: string; href: string };
function Hero() {
  const reducedMotion = useReducedMotion();
  const { session } = useApp();
  const home = useQuery({
    queryKey: ["home"],
    queryFn: () => api<{ banners: Banner[] }>("/home"),
  });
  const [current, setCurrent] = useState(0);
  const banners = home.data?.banners || [];
  useEffect(() => {
    if (banners.length < 2 || reducedMotion) return;
    const t = setInterval(
      () => setCurrent((c) => (c + 1) % banners.length),
      7000,
    );
    return () => clearInterval(t);
  }, [banners.length, reducedMotion]);
  const banner = banners[current % banners.length];
  return (
    <section className="hero" aria-label="精选专题">
      <AnimatePresence initial={false}>
        <motion.div
          key={banner?.image || "empty"}
          className="hero-frame"
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          exit={{ opacity: 0 }}
          transition={{ duration: reducedMotion ? 0 : 0.65 }}
        >
          {banner ? (
            <SmartImage
              src={banner.image}
              alt="雪山与湖泊的自然风景"
              className="hero-image"
              fetchPriority="high"
            />
          ) : (
            <div className="hero-placeholder">
              <Compass size={220} />
            </div>
          )}
          <div className="hero-overlay" />
          <div className="hero-copy">
            <motion.h1
              initial={{ y: 12, opacity: 0 }}
              animate={{ y: 0, opacity: 1 }}
              transition={{ delay: 0.15 }}
            >
              {banner?.title || "让每一次打开屏幕\n都是一次新的出发"}
            </motion.h1>
            <p>{banner?.subtitle || "发现优质壁纸 · 收藏日常灵感"}</p>
            <Link
              className="primary hero-button"
              to={banner?.href || "/discover"}
            >
              探索更多 <ArrowRight size={17} />
            </Link>
          </div>
        </motion.div>
      </AnimatePresence>
      {banners.length > 1 && (
        <div className="hero-dots">
          {banners.map((_, i) => (
            <button
              key={i}
              aria-label={"切换横幅 " + (i + 1)}
              aria-current={i === current}
              onClick={() => setCurrent(i)}
            />
          ))}
        </div>
      )}
      {session?.demo && <span className="demo-flag">灵感预览</span>}
    </section>
  );
}
export function HomePage() {
  const [p] = useSearchParams();
  return (
    <>
      <Hero />
      <div className="home-content">
        <div className="home-main">
          <Filters />
          <SectionTitle
            title="精选壁纸"
            subtitle="发现世界的不同美好"
            more="/discover"
          />
          <WallpaperGrid
            filters={{
              featured: "prefer",
              topic: p.get("topic") || "",
              orientation: p.get("orientation") || "",
            }}
          />
        </div>
        <Sidebars />
      </div>
    </>
  );
}
export function DiscoverPage() {
  const [p, setP] = useSearchParams();
  const { session, login } = useApp();
  return (
    <>
      <div className="page-heading">
        <div>
          <span className="eyebrow">EXPLORE YOUR WORLD</span>
          <h1>{p.get("q") ? "搜索「" + p.get("q") + "」" : "发现新的心动"}</h1>
          <p>山川、城市与想象，找到属于你的那一帧。</p>
        </div>
        <Compass className="heading-art" size={70} />
      </div>
      <div className="tabs compact">
        <button
          className={!p.get("scope") ? "active" : ""}
          onClick={() =>
            setP((prev) => {
              const n = new URLSearchParams(prev);
              n.delete("scope");
              return n;
            })
          }
        >
          全部发现
        </button>
        <button
          className={p.get("scope") === "subscriptions" ? "active" : ""}
          onClick={() => {
            if (!session?.user) {
              login();
              return;
            }
            setP((prev) => {
              const n = new URLSearchParams(prev);
              n.set("scope", "subscriptions");
              return n;
            });
          }}
        >
          我的订阅
        </button>
      </div>
      <Filters />
      {p.get("q") && (
        <div className="search-summary">
          <Search size={15} />
          与标题、作者或标签相关的壁纸{" "}
          <button
            className="text-btn"
            onClick={() =>
              setP((prev) => {
                const n = new URLSearchParams(prev);
                n.delete("q");
                return n;
              })
            }
          >
            清除搜索
          </button>
        </div>
      )}
      <WallpaperGrid
        filters={{
          q: p.get("q") || "",
          topic: p.get("topic") || "",
          tag: p.get("tag") || "",
          orientation: p.get("orientation") || "",
          scope: p.get("scope") || "",
        }}
        emptyText={
          p.get("scope")
            ? "先订阅喜欢的作者、主题或渠道，新的作品会在这里相遇。"
            : undefined
        }
      />
    </>
  );
}
export function DetailPage() {
  const { id } = useParams(),
    { guard, notify, session } = useApp(),
    client = useQueryClient();
  const [selected, setSelected] = useState(0),
    [full, setFull] = useState(false),
    [downloading, setDownloading] = useState(false),
    [busy, setBusy] = useState(false),
    [relatedOffset, setRelatedOffset] = useState(0);
  const viewed = useRef("");
  const detail = useQuery({
    queryKey: ["wallpaper", id],
    queryFn: () => api<Wallpaper>("/wallpapers/" + id),
  });
  const w = detail.data;
  const author = useQuery({
    queryKey: ["entity", w?.author.id],
    queryFn: () => api<Entity>("/entities/" + w!.author.id),
    enabled: !!w?.author.id,
  });
  const related = useQuery({
    queryKey: ["related", id, w?.topicIds[0]],
    queryFn: () =>
      api<Page<Wallpaper>>(
        "/wallpapers?" + params({ topic: w?.topicIds[0], limit: "30" }),
      ),
    enabled: !!w,
  });
  useEffect(() => {
    setSelected(0);
    setFull(false);
    setRelatedOffset(0);
  }, [id]);
  useEffect(() => {
    if (w && session?.user && viewed.current !== w.id + session.user.id) {
      viewed.current = w.id + session.user.id;
      api("/wallpapers/" + w.id + "/history", "POST")
        .then(() => client.invalidateQueries({ queryKey: ["session"] }))
        .catch(() => {});
    }
  }, [w, session?.user, client]);
  useEffect(() => {
    if (!full) return;
    const handler = (e: KeyboardEvent) => {
      if (e.key === "Escape") setFull(false);
      if (e.key === "ArrowRight")
        setSelected((s) => (s + 1) % (w?.images.length || 1));
      if (e.key === "ArrowLeft")
        setSelected(
          (s) => (s - 1 + (w?.images.length || 1)) % (w?.images.length || 1),
        );
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [full, w]);
  if (detail.isLoading) return <Loading count={4} />;
  if (detail.error)
    return <ErrorState error={detail.error} retry={() => detail.refetch()} />;
  if (!w) return null;
  const img = w.images[selected];
  if (!img) return <Empty title="图片暂不可用" />;
  const interact = (kind: string, on: boolean) =>
    guard(async () => {
      setBusy(true);
      try {
        await api(
          `/wallpapers/${w.id}/interactions/${kind}`,
          on ? "DELETE" : "PUT",
        );
        await client.invalidateQueries();
        notify(
          on
            ? "已取消"
            : kind === "favorite"
              ? "已加入我的收藏"
              : "谢谢你的喜欢",
        );
      } finally {
        setBusy(false);
      }
    });
  const recommended = related.data?.items.filter((r) => r.id !== w.id) || [];
  return (
    <>
      <div className="breadcrumb">
        <Link to="/">首页</Link>
        <ChevronRight size={13} />
        <Link to={"/discover?topic=" + w.topicIds[0]}>精选壁纸</Link>
        <ChevronRight size={13} />
        <span>{w.title}</span>
      </div>
      <div className="detail-layout">
        <div className="gallery">
          <div className="gallery-main">
            <SmartImage src={img.url} alt={w.title} />
            <button
              className="gallery-expand"
              aria-label="全屏预览"
              onClick={() => setFull(true)}
            >
              <Expand size={20} />
            </button>
            {w.images.length > 1 && (
              <button
                className="gallery-next"
                aria-label="下一张图片"
                onClick={() => setSelected((selected + 1) % w.images.length)}
              >
                <ChevronRight />
              </button>
            )}
            <span className="image-counter">
              {selected + 1}/{w.images.length}
            </span>
          </div>
          <div className="thumbnails">
            {w.images.map((m, i) => (
              <button
                key={m.id}
                aria-label={"查看第 " + (i + 1) + " 张"}
                aria-pressed={selected === i}
                className={selected === i ? "selected" : ""}
                onClick={() => setSelected(i)}
              >
                <SmartImage src={m.thumbnail} alt="" />
              </button>
            ))}
          </div>
        </div>
        <aside className="detail-info">
          <h1>
            {w.title}{" "}
            <span className="quality-badge">
              {Math.max(img.width, img.height) >= 3840 ? "4K" : "HD"}
            </span>
          </h1>
          <p className="detail-description">{w.description}</p>
          <div className="detail-author">
            <Link to={"/authors/" + w.author.id}>
              <Avatar entity={w.author} />
              <span>
                <strong>{w.author.name}</strong>
                <small>{number(author.data?.subscribers)} 位关注者</small>
              </span>
            </Link>
            {author.data && <Subscribe entity={author.data} />}
          </div>
          <dl className="metadata">
            <div>
              <dt>
                <Globe size={15} />
                来源渠道
              </dt>
              <dd>
                <Link to={"/channels/" + w.channel.id}>{w.channel.name}</Link>
              </dd>
            </div>
            <div>
              <dt>
                <Image size={15} />
                分辨率
              </dt>
              <dd>
                {img.width} × {img.height}
              </dd>
            </div>
            <div>
              <dt>
                <HardDrive size={15} />
                文件大小
              </dt>
              <dd>{(img.bytes / 1024 / 1024).toFixed(1)} MB</dd>
            </div>
            <div>
              <dt>
                <CalendarDays size={15} />
                发布于
              </dt>
              <dd>{new Date(w.publishedAt).toLocaleDateString("zh-CN")}</dd>
            </div>
          </dl>
          <div className="tags">
            {w.tags.map((t, i) => (
              <Link key={t + i} to={"/discover?tag=" + encodeURIComponent(t)}>
                # {t}
              </Link>
            ))}
          </div>
          <div className="detail-buttons">
            <button
              className="primary"
              disabled={downloading}
              onClick={() =>
                guard(async () => {
                  setDownloading(true);
                  try {
                    await downloadFile(w, img);
                    await client.invalidateQueries({ queryKey: ["session"] });
                    notify("已响应原图下载，请在浏览器中查看");
                  } finally {
                    setDownloading(false);
                  }
                })
              }
            >
              <ArrowDownToLine size={20} />
              {downloading ? "准备原图…" : "下载"}
            </button>
            <button
              className={"secondary " + (w.favorited ? "selected" : "")}
              disabled={busy}
              onClick={() => interact("favorite", w.favorited)}
            >
              <Heart size={20} fill={w.favorited ? "currentColor" : "none"} />
              {w.favorited ? "已收藏" : "收藏"}
            </button>
          </div>
          <div className="detail-social">
            <button
              className={w.liked ? "liked" : ""}
              disabled={busy}
              onClick={() => interact("like", w.liked)}
              aria-label="喜欢壁纸"
            >
              <Heart size={20} fill={w.liked ? "currentColor" : "#ff6680"} />
              {number(w.likes)}
            </button>
            <button
              onClick={() => notify("评论功能即将开放，敬请期待")}
              aria-label="评论"
            >
              <MessageCircle size={19} />
              <span>评论</span>
            </button>
            <button
              onClick={async () => {
                try {
                  if (navigator.share)
                    await navigator.share({
                      title: w.title,
                      url: location.href,
                    });
                  else {
                    await navigator.clipboard.writeText(location.href);
                    notify("链接已复制");
                  }
                } catch {
                  notify("分享未完成，可复制浏览器地址分享");
                }
              }}
            >
              <Share2 size={19} />
              分享
            </button>
            <button
              aria-label="更多信息"
              onClick={() =>
                notify(
                  "收藏 " +
                    number(w.favorites) +
                    " 次 · 下载请求 " +
                    number(w.downloads) +
                    " 次",
                )
              }
            >
              <MoreHorizontal size={21} />
            </button>
          </div>
          {w.sourceUrl && (
            <div className="source-note">
              <a href={w.sourceUrl} target="_blank" rel="noreferrer">
                查看 X 原帖 ↗
              </a>
              {w.sourceMetrics.likes !== undefined && (
                <span>
                  原帖点赞 {number(w.sourceMetrics.likes)} · 不计入站内热度
                </span>
              )}
            </div>
          )}
        </aside>
      </div>
      <section className="related">
        <SectionTitle title="相关推荐">
          <button
            className="text-btn"
            onClick={() =>
              setRelatedOffset((o) =>
                recommended.length ? (o + 5) % recommended.length : 0,
              )
            }
          >
            <RefreshCw size={15} />
            换一批
          </button>
        </SectionTitle>
        <div className="related-grid">
          {[...recommended, ...recommended]
            .slice(
              relatedOffset,
              relatedOffset + Math.min(5, recommended.length),
            )
            .map((r) => (
              <Link key={r.id} to={"/wallpaper/" + r.id}>
                <SmartImage src={r.images[0]?.thumbnail} alt={r.title} />
                <span>{r.title}</span>
              </Link>
            ))}
        </div>
        {!recommended.length && (
          <Empty title="更多灵感正在路上" text="稍后再来发现新的风景。" />
        )}
      </section>
      <AnimatePresence>
        {full && (
          <motion.div
            role="dialog"
            aria-label="全屏图片预览"
            aria-modal="true"
            className="lightbox"
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            onClick={() => setFull(false)}
          >
            <button
              className="lightbox-close"
              aria-label="关闭预览"
              onClick={() => setFull(false)}
            >
              <X />
            </button>
            <button
              className="lightbox-prev"
              aria-label="上一张"
              onClick={(e) => {
                e.stopPropagation();
                setSelected((selected - 1 + w.images.length) % w.images.length);
              }}
            >
              <ChevronLeft />
            </button>
            <SmartImage
              src={img.url}
              alt={w.title}
              onClick={(e) => e.stopPropagation()}
            />
            <button
              className="lightbox-next"
              aria-label="下一张"
              onClick={(e) => {
                e.stopPropagation();
                setSelected((selected + 1) % w.images.length);
              }}
            >
              <ChevronRight />
            </button>
            <span>
              {w.title} · {selected + 1}/{w.images.length}
            </span>
          </motion.div>
        )}
      </AnimatePresence>
    </>
  );
}
export function RankingPage() {
  const [kind, setKind] = useState("wallpaper"),
    [period, setPeriod] = useState("7d");
  const { notify } = useApp();
  const q = useQuery({
    queryKey: ["ranks", kind, period],
    queryFn: () =>
      api<Page<Wallpaper & Entity>>("/ranks?" + params({ kind, period })),
  });
  const top = q.data?.items[0];
  return (
    <>
      <div className="page-heading ranking-heading">
        <div>
          <h1>排行榜</h1>
          <p>发现最受欢迎的壁纸、创作者与优质渠道</p>
        </div>
        <div className="ranking-motto">
          用热爱，点亮每一张屏幕
          <br />
          发现全站最受欢迎的视觉作品
          <Crown size={75} />
        </div>
      </div>
      <div className="ranking-shell">
        <div className="tabs rank-tabs">
          {[
            ["wallpaper", "热门壁纸"],
            ["author", "热门作者"],
            ["channel", "热门渠道"],
          ].map(([v, t]) => (
            <button
              className={v === kind ? "active" : ""}
              key={v}
              onClick={() => setKind(v)}
            >
              {t}
            </button>
          ))}
          <select
            aria-label="排行周期"
            value={period}
            onChange={(e) => setPeriod(e.target.value)}
          >
            <option value="7d">近 7 天</option>
            <option value="30d">近 30 天</option>
            <option value="all">全部时间</option>
          </select>
        </div>
        <div className="ranking-body">
          <div className="ranking-table">
            {q.isLoading ? (
              <Loading count={3} />
            ) : q.error ? (
              <ErrorState error={q.error} retry={() => q.refetch()} />
            ) : (
              <>
                <div className="ranking-row table-head">
                  <span>排名</span>
                  <span>预览</span>
                  <span>{kind === "wallpaper" ? "图名" : "名称"}</span>
                  <span>{kind === "wallpaper" ? "作者" : "订阅"}</span>
                  <span>热度</span>
                </div>
                {q.data?.items.map((item, i) => (
                  <div className="ranking-row" key={item.id}>
                    <span className={"place place-" + i}>
                      {i < 3 ? <Crown size={24} fill="currentColor" /> : i + 1}
                      {i < 3 && <b>{i + 1}</b>}
                    </span>
                    <Link
                      to={
                        kind === "wallpaper"
                          ? "/wallpaper/" + item.id
                          : kind === "author"
                            ? "/authors/" + item.id
                            : "/channels/" + item.id
                      }
                    >
                      <SmartImage
                        src={
                          kind === "wallpaper"
                            ? item.images[0]?.thumbnail
                            : item.avatar || item.cover
                        }
                        alt=""
                      />
                    </Link>
                    <Link
                      className="rank-title"
                      to={
                        kind === "wallpaper"
                          ? "/wallpaper/" + item.id
                          : kind === "author"
                            ? "/authors/" + item.id
                            : "/channels/" + item.id
                      }
                    >
                      <strong>{item.title || item.name}</strong>
                      <small>{item.description}</small>
                    </Link>
                    <div className="rank-author">
                      {kind === "wallpaper" ? (
                        <Link to={"/authors/" + item.author.id}>
                          <Avatar entity={item.author} />
                          <span>{item.author.name}</span>
                        </Link>
                      ) : (
                        <Subscribe entity={item} />
                      )}
                    </div>
                    <span className="rank-score">{number(item.score)}</span>
                  </div>
                ))}
                {!q.data?.items.length && (
                  <Empty
                    title="等你发现第一份美好"
                    text="作品发布后会出现在榜单中。"
                  />
                )}
              </>
            )}
          </div>
          <aside className="ranking-sidebar">
            {top && kind === "wallpaper" && (
              <section className="weekly">
                <h3>本期人气作品</h3>
                <p>
                  {period === "7d"
                    ? "最近 7 天"
                    : period === "30d"
                      ? "最近 30 天"
                      : "全部时间"}
                </p>
                <Link to={"/wallpaper/" + top.id}>
                  <div className="weekly-img">
                    <SmartImage
                      src={top.images[0]?.thumbnail}
                      alt={top.title}
                    />
                    <Crown size={38} fill="#ffca67" />
                  </div>
                  <h3>{top.title}</h3>
                </Link>
                <span className="weekly-score">
                  <Crown size={18} />
                  {number(top.score)} 热度
                </span>
              </section>
            )}
            <section className="join-panel">
              <h3>加入壁纸星球</h3>
              <p>获取更多高质量壁纸，支持你喜欢的创作者</p>
              <button
                className="primary full"
                onClick={() => notify("会员功能即将开放，敬请期待")}
              >
                立即开通
              </button>
            </section>
            <p className="rank-explainer">
              热度 = 点赞 + 收藏 × 4 + 下载 × 2<br />
              每小时更新 · 仅统计站内互动
            </p>
          </aside>
        </div>
      </div>
    </>
  );
}
export function EntitiesPage({ kind }: { kind: "author" | "topic" }) {
  const query = useInfiniteQuery({
    queryKey: ["entities-list", kind],
    initialPageParam: "",
    queryFn: ({ pageParam }) =>
      api<Page<Entity>>("/entities?" + params({ kind, cursor: pageParam })),
    getNextPageParam: (p) => p.nextCursor || undefined,
  });
  return (
    <>
      <div className="page-heading">
        <div>
          <span className="eyebrow">
            {kind === "author"
              ? "MEET THE CREATORS"
              : "COLLECTIONS OF INSPIRATION"}
          </span>
          <h1>
            {kind === "author" ? "与有趣的灵魂相遇" : "每一种热爱，都有归处"}
          </h1>
          <p>
            {kind === "author"
              ? "关注喜欢的创作者，把灵感留在日常。"
              : "订阅你喜欢的主题，探索更广阔的视觉世界。"}
          </p>
        </div>
      </div>
      {query.isLoading ? (
        <Loading count={6} />
      ) : query.error ? (
        <ErrorState error={query.error} retry={() => query.refetch()} />
      ) : (
        <div
          className={"entity-grid " + (kind === "author" ? "author-grid" : "")}
        >
          {query.data?.pages
            .flatMap((p) => p.items)
            .map((e) => (
              <article className="entity-card" key={e.id}>
                {kind === "topic" && (
                  <Link to={"/topics/" + e.id}>
                    <SmartImage src={e.cover} alt={e.name} />
                  </Link>
                )}
                <div className="entity-card-body">
                  <Link
                    to={(kind === "author" ? "/authors/" : "/topics/") + e.id}
                  >
                    {kind === "author" && <Avatar entity={e} size="large" />}
                    <h2>{e.name}</h2>
                    <p>{e.description}</p>
                    <small>
                      {number(e.count)} 张作品 · {number(e.subscribers)}{" "}
                      位订阅者
                    </small>
                  </Link>
                  <Subscribe entity={e} />
                </div>
              </article>
            ))}
        </div>
      )}
      {query.hasNextPage && (
        <div className="load-more">
          <button
            className="secondary"
            disabled={query.isFetchingNextPage}
            onClick={() => query.fetchNextPage()}
          >
            加载更多
          </button>
        </div>
      )}
      {query.data?.pages[0].items.length === 0 && (
        <Empty title="创作者即将到来" text="新的灵感，正在路上。" />
      )}
    </>
  );
}
export function EntityPage() {
  const { id } = useParams();
  const query = useQuery({
    queryKey: ["entity", id],
    queryFn: () => api<Entity>("/entities/" + id),
  });
  if (query.isLoading) return <Loading count={3} />;
  if (query.error)
    return <ErrorState error={query.error} retry={() => query.refetch()} />;
  const e = query.data!;
  return (
    <>
      <div className="entity-heading">
        {e.cover && (
          <SmartImage src={e.cover} className="entity-heading-cover" alt="" />
        )}
        <div className="entity-heading-content">
          <Avatar entity={e} size="large" />
          <div>
            <span className="eyebrow">
              {e.kind === "channel"
                ? "CHANNEL"
                : e.kind === "author"
                  ? "CREATOR"
                  : "COLLECTION"}
            </span>
            <h1>{e.name}</h1>
            <p>{e.description}</p>
            <small>
              {number(e.count)} 张作品 · {number(e.subscribers)} 位订阅者
            </small>
          </div>
          <Subscribe entity={e} />
        </div>
      </div>
      <SectionTitle title="全部作品" />
      <WallpaperGrid filters={{ [e.kind]: e.id }} />
    </>
  );
}

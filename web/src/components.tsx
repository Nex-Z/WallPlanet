import { useEffect, useState, useRef, type ReactNode } from "react";
import { Link, NavLink, useNavigate, useSearchParams } from "react-router-dom";
import {
  useInfiniteQuery,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import {
  Bell,
  Search,
  Heart,
  ArrowDownToLine,
  ChevronRight,
  Plus,
  Check,
  Home,
  ChartNoAxesColumn,
  UserRound,
  ImageOff,
  LoaderCircle,
  ArrowUp,
  SlidersHorizontal,
  Crown,
  Menu,
  Orbit,
} from "lucide-react";
import {
  api,
  number,
  params,
  type Entity,
  type Wallpaper,
  type Page,
} from "./api";
import { useApp } from "./context";
import { Masonry } from "./masonry";
export function SmartImage({
  src,
  alt = "",
  className = "",
  ...rest
}: React.ImgHTMLAttributes<HTMLImageElement>) {
  const [bad, setBad] = useState(false);
  useEffect(() => setBad(false), [src]);
  return bad || !src ? (
    <div
      className={"image-fallback " + className}
      role="img"
      aria-label={alt || "暂无图片"}
    >
      <ImageOff size={24} />
      <span>暂时没有图片</span>
    </div>
  ) : (
    <img
      {...rest}
      className={className}
      src={src}
      alt={alt}
      onError={() => setBad(true)}
    />
  );
}
export function Avatar({
  entity,
  size = "",
}: {
  entity?: { avatar?: string; name?: string };
  size?: string;
}) {
  return entity?.avatar ? (
    <SmartImage
      className={"avatar " + size}
      src={entity.avatar}
      alt={entity.name}
    />
  ) : (
    <span className={"avatar avatar-letter " + size}>
      {entity?.name?.slice(0, 1) || <UserRound size={19} />}
    </span>
  );
}
export function Header() {
  const { session, login, notify } = useApp(),
    navigate = useNavigate();
  const [search, setSearch] = useState(""),
    [menu, setMenu] = useState(false);
  return (
    <>
      <header className="site-header">
        <div className="header-inner">
          <Link className="brand" to="/">
            <img src="/logo.svg" alt="" />
            <span>
              <strong>壁纸星球</strong>
              <small>发现更美的世界</small>
            </span>
          </Link>
          <nav
            className={menu ? "desktop-nav expanded" : "desktop-nav"}
            aria-label="主导航"
          >
            {[
              ["/", "首页"],
              ["/discover", "发现"],
              ["/ranking", "排行"],
              ["/authors", "作者"],
              ["/topics", "专题"],
            ].map(([to, label]) => (
              <NavLink
                to={to}
                key={to}
                end={to === "/"}
                onClick={() => setMenu(false)}
              >
                {label}
              </NavLink>
            ))}
          </nav>
          <form
            className="header-search"
            role="search"
            onSubmit={(e) => {
              e.preventDefault();
              navigate("/discover?" + params({ q: search }));
            }}
          >
            <Search size={17} />
            <input
              aria-label="搜索壁纸、作者或标签"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder="搜索壁纸、作者或标签…"
            />
          </form>
          <div className="header-actions">
            <button
              className="icon-btn notification"
              aria-label="通知"
              onClick={() => notify("站内通知即将开放，敬请期待")}
            >
              <Bell size={21} />
            </button>
            {session?.user ? (
              <Link to="/me" aria-label="我的主页">
                <Avatar entity={session.user.profile} />
              </Link>
            ) : (
              <button className="icon-btn" aria-label="登录" onClick={login}>
                <UserRound size={21} />
              </button>
            )}
            <button
              className="primary member-btn"
              onClick={() => notify("会员功能即将开放，敬请期待")}
            >
              开通会员
            </button>
            <button
              className="icon-btn mobile-menu"
              aria-label="打开菜单"
              onClick={() => setMenu(!menu)}
            >
              <Menu />
            </button>
          </div>
        </div>
      </header>
      <nav className="mobile-bottom" aria-label="移动导航">
        {[
          ["/", "首页", Home],
          ["/ranking", "排行", ChartNoAxesColumn],
          ["/me/favorites", "收藏", Heart],
          ["/me", "我的", UserRound],
        ].map(([to, label, Icon]) => {
          const I = Icon as typeof Home;
          return (
            <NavLink end to={to as string} key={to as string}>
              <I size={22} />
              <span>{label as string}</span>
            </NavLink>
          );
        })}
      </nav>
    </>
  );
}
export function Footer() {
  const { session } = useApp();
  return (
    <footer className="site-footer">
      <span>
        壁纸星球 <b>·</b> 发现美，收藏美，每天更美一点
      </span>
      <span>
        {session?.demo && <em>演示图库</em>}
        {session?.user?.role === "admin" && <Link to="/admin">管理后台</Link>}
        <button
          className="text-btn"
          onClick={() => window.scrollTo({ top: 0, behavior: "smooth" })}
        >
          回到顶部 <ArrowUp size={13} />
        </button>
      </span>
    </footer>
  );
}
export function SectionTitle({
  title,
  subtitle,
  more,
  children,
}: {
  title: string;
  subtitle?: string;
  more?: string;
  children?: ReactNode;
}) {
  return (
    <div className="section-title">
      <div>
        <h2>{title}</h2>
        {subtitle && <p>{subtitle}</p>}
      </div>
      {children}
      {more && (
        <Link className="more" to={more}>
          更多 <ChevronRight size={14} />
        </Link>
      )}
    </div>
  );
}
export function Empty({
  title = "还没有内容",
  text = "美好的事物值得等待。",
  action,
}: {
  title?: string;
  text?: string;
  action?: ReactNode;
}) {
  return (
    <div className="empty">
      <Orbit size={42} />
      <h3>{title}</h3>
      <p>{text}</p>
      {action}
    </div>
  );
}
export function ErrorState({
  error,
  retry,
}: {
  error: Error;
  retry: () => void;
}) {
  return (
    <Empty
      title="暂时无法加载"
      text={error.message}
      action={
        <button className="secondary" onClick={retry}>
          重新加载
        </button>
      }
    />
  );
}
export function Loading({ count = 10 }: { count?: number }) {
  return (
    <div className="skeleton-grid">
      {Array.from({ length: count }, (_, i) => (
        <div
          key={i}
          className="skeleton"
          style={{ height: 180 + (i % 3) * 45 }}
        />
      ))}
    </div>
  );
}
export function Subscribe({ entity }: { entity: Entity }) {
  const { guard, notify } = useApp(),
    client = useQueryClient();
  const [busy, setBusy] = useState(false);
  return (
    <button
      disabled={busy}
      className={"subscribe " + (entity.subscribed ? "subscribed" : "")}
      onClick={() =>
        guard(async () => {
          setBusy(true);
          try {
            await api(
              `/entities/${entity.id}/subscription`,
              entity.subscribed ? "DELETE" : "PUT",
            );
            await client.invalidateQueries();
            notify(entity.subscribed ? "已取消订阅" : "订阅成功");
          } finally {
            setBusy(false);
          }
        })
      }
    >
      {entity.subscribed ? <Check size={13} /> : <Plus size={13} />}{" "}
      {entity.subscribed
        ? "已订阅"
        : entity.kind === "author"
          ? "关注"
          : "订阅"}
    </button>
  );
}
export function WallpaperCard({
  w,
  index = 0,
}: {
  w: Wallpaper;
  index?: number;
}) {
  const { guard, notify } = useApp(),
    client = useQueryClient();
  const [busy, setBusy] = useState(false);
  const img = w.images[0];
  return (
    <article className="wallpaper-card">
      <Link to={"/wallpaper/" + w.id} aria-label={w.title}>
        <SmartImage
          loading={index < 5 ? "eager" : "lazy"}
          src={img?.thumbnail}
          alt={w.title}
          style={{
            aspectRatio: [
              0.78, 0.64, 0.83, 0.73, 1.04, 0.76, 1.35, 0.65, 0.8, 0.77,
            ][index % 10],
          }}
        />
        <div className="card-shade">
          <span>{w.title}</span>
          <small>{w.author.name}</small>
        </div>
      </Link>
      <button
        aria-label={(w.favorited ? "取消收藏 " : "收藏 ") + w.title}
        disabled={busy}
        className={"card-like " + (w.favorited ? "is-liked" : "")}
        onClick={() =>
          guard(async () => {
            setBusy(true);
            try {
              await api(
                `/wallpapers/${w.id}/interactions/favorite`,
                w.favorited ? "DELETE" : "PUT",
              );
              await client.invalidateQueries({ queryKey: ["wallpapers"] });
              await client.invalidateQueries({ queryKey: ["session"] });
              notify(w.favorited ? "已取消收藏" : "已加入我的收藏");
            } finally {
              setBusy(false);
            }
          })
        }
      >
        <Heart size={13} fill="currentColor" />
        {number(w.favorites)}
      </button>
      <span className="card-resolution">
        {img?.width >= 3840 || img?.height >= 3840 ? "4K" : "HD"}
      </span>
    </article>
  );
}
export function WallpaperGrid({
  filters = {},
  emptyText,
}: {
  filters?: Record<string, string | undefined>;
  emptyText?: string;
}) {
  const query = useInfiniteQuery({
    queryKey: ["wallpapers", filters],
    initialPageParam: "",
    queryFn: ({ pageParam }) =>
      api<Page<Wallpaper>>(
        "/wallpapers?" + params({ ...filters, cursor: pageParam }),
      ),
    getNextPageParam: (p) => p.nextCursor || undefined,
  });
  if (query.isLoading) return <Loading />;
  if (query.error)
    return <ErrorState error={query.error} retry={() => query.refetch()} />;
  const items = query.data?.pages.flatMap((p) => p.items) || [];
  if (!items.length)
    return (
      <Empty
        title={filters.q ? "没有找到相关壁纸" : "这里等待新的风景"}
        text={emptyText || "试试其他分类，或稍后回来看看。"}
      />
    );
  return (
    <>
      <Masonry items={items} />
      {query.hasNextPage && (
        <div className="load-more">
          <button
            className="secondary"
            disabled={query.isFetchingNextPage}
            onClick={() => query.fetchNextPage()}
          >
            {query.isFetchingNextPage ? (
              <LoaderCircle className="spin" size={16} />
            ) : null}
            加载更多壁纸
          </button>
        </div>
      )}
    </>
  );
}
export function Filters() {
  const [p, setP] = useSearchParams();
  const topics = useQuery({
    queryKey: ["entities", "topic"],
    queryFn: () => api<Page<Entity>>("/entities?kind=topic"),
  });
  const set = (k: string, v: string) =>
    setP((prev) => {
      const n = new URLSearchParams(prev);
      v ? n.set(k, v) : n.delete(k);
      return n;
    });
  return (
    <div className="filters">
      <div className="category-scroll">
        <button
          className={!p.get("topic") ? "chip active" : "chip"}
          onClick={() => set("topic", "")}
        >
          全部
        </button>
        {[...(topics.data?.items || [])]
          .sort((a, b) => {
            const order = [
              "nature",
              "anime",
              "city",
              "minimal",
              "tech",
              "car",
              "space",
              "art",
            ];
            const ai = order.indexOf(a.id),
              bi = order.indexOf(b.id);
            return (ai < 0 ? 99 : ai) - (bi < 0 ? 99 : bi);
          })
          .map((t) => (
            <button
              key={t.id}
              className={"chip " + (p.get("topic") === t.id ? "active" : "")}
              onClick={() => set("topic", t.id)}
            >
              {t.name}
            </button>
          ))}
      </div>
      <label className="orientation">
        <SlidersHorizontal size={14} />
        <select
          aria-label="图片方向"
          value={p.get("orientation") || ""}
          onChange={(e) => set("orientation", e.target.value)}
        >
          <option value="">全部方向</option>
          <option value="landscape">横屏壁纸</option>
          <option value="portrait">竖屏壁纸</option>
        </select>
      </label>
    </div>
  );
}
export function Sidebars() {
  const ranks = useQuery({
      queryKey: ["ranks", "side"],
      queryFn: () => api<Page<Wallpaper>>("/ranks?kind=wallpaper&period=7d"),
    }),
    authors = useQuery({
      queryKey: ["entities", "author"],
      queryFn: () => api<Page<Entity>>("/entities?kind=author"),
    });
  return (
    <aside className="home-sidebar">
      <section className="side-panel">
        <SectionTitle title="热门榜单" more="/ranking" />
        {ranks.data?.items.slice(0, 5).map((w, i) => (
          <Link className="mini-rank" key={w.id} to={"/wallpaper/" + w.id}>
            <b className={"rank-badge rank-" + i}>{i + 1}</b>
            <SmartImage src={w.images[0]?.thumbnail} alt="" />
            <strong>{w.title}</strong>
            <span>{number(w.score)}</span>
          </Link>
        ))}
        {!ranks.data?.items.length && (
          <p className="muted small">等待第一张热门壁纸</p>
        )}
      </section>
      <section className="side-panel">
        <SectionTitle title="推荐作者" more="/authors" />
        {authors.data?.items
          .filter((a) => a.featured)
          .slice(0, 4)
          .map((a) => (
            <div className="mini-author" key={a.id}>
              <Link to={"/authors/" + a.id}>
                <Avatar entity={a} />
                <span>
                  <strong>{a.name}</strong>
                  <small>{number(a.subscribers)} 位关注者</small>
                </span>
              </Link>
              <Subscribe entity={a} />
            </div>
          ))}
        {!authors.data?.items.length && (
          <p className="muted small">新的创作者即将到来</p>
        )}
      </section>
      <section className="side-invite">
        <Crown size={30} />
        <h3>让美好，常伴左右</h3>
        <p>
          订阅喜欢的作者
          <br />
          不错过每一次灵感更新
        </p>
        <Link className="secondary" to="/topics">
          发现更多灵感 <ChevronRight size={14} />
        </Link>
      </section>
    </aside>
  );
}

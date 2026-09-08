import { useState } from "react";
import { Link, NavLink, useNavigate, useParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import {
  Home,
  Heart,
  ArrowDownToLine,
  Clock3,
  Bookmark,
  Settings,
  ChevronRight,
  LogOut,
  ShieldCheck,
  Check,
  X,
  UserRound,
} from "lucide-react";
import {
  api,
  number,
  type Entity,
  type Page,
  type Profile,
  type Wallpaper,
} from "./api";
import { useApp } from "./context";
import {
  Avatar,
  Empty,
  ErrorState,
  Loading,
  SectionTitle,
  SmartImage,
  Subscribe,
  WallpaperGrid,
} from "./components";
const tabs = [
  ["", "我的首页", Home],
  ["favorites", "我的收藏", Heart],
  ["downloads", "我的下载", ArrowDownToLine],
  ["history", "浏览历史", Clock3],
  ["subscriptions", "我的订阅", Bookmark],
  ["settings", "账号设置", Settings],
] as const;
export function MyPage() {
  const { tab = "" } = useParams();
  const { session, loading, login, notify, refresh } = useApp();
  const navigate = useNavigate();
  const [editing, setEditing] = useState(false);
  if (loading) return <Loading count={4} />;
  const user = session?.user;
  if (!user)
    return (
      <Empty
        title="你的壁纸世界，从这里开始"
        text="登录后收藏喜欢的图片、关注创作者，留下每一次心动。"
        action={
          <button className="primary" onClick={login}>
            登录 / 注册
          </button>
        }
      />
    );
  return (
    <div className="profile-layout">
      <aside className="profile-sidebar">
        <div className="profile-person">
          <Avatar entity={user.profile} size="profile-avatar" />
          <h2>{user.profile.name || user.username}</h2>
          <p>{user.profile.bio || "热爱生活，也热爱眼前的风景"}</p>
          <div className="profile-stats">
            <span>
              <b>{number(user.stats.subscriptions)}</b>订阅
            </span>
            <span>
              <b>{number(user.stats.favorites)}</b>收藏
            </span>
            <span>
              <b>{number(user.stats.downloads)}</b>下载
            </span>
          </div>
        </div>
        <nav>
          {tabs.map(([key, label, Icon]) => (
            <NavLink end key={key} to={"/me" + (key ? "/" + key : "")}>
              <Icon size={19} />
              {label}
            </NavLink>
          ))}
          {user.role === "admin" && (
            <Link to="/admin">
              <ShieldCheck size={19} />
              管理后台
            </Link>
          )}
          <button
            onClick={async () => {
              try {
                await api("/auth/logout", "POST");
                await refresh();
                navigate("/");
                notify("已退出登录");
              } catch (e) {
                notify((e as Error).message);
              }
            }}
          >
            <LogOut size={18} />
            退出登录
          </button>
        </nav>
      </aside>
      <div className="profile-content">
        {!tab && (
          <>
            <div className="profile-cover">
              {user.profile.cover ? (
                <SmartImage src={user.profile.cover} alt="个人主页背景" />
              ) : (
                <SmartImage src="/assets/mountain.jpg" alt="雪山风景" />
              )}
              <div className="profile-cover-shade" />
              <div>
                <strong>好的壁纸</strong>
                <p>让平凡的日子也闪闪发光</p>
                <span>Keep A Good Mood.</span>
                <button onClick={() => setEditing(true)}>编辑资料</button>
              </div>
            </div>
            <div className="quick-stats">
              {[
                ["favorites", "我的收藏", Heart, user.stats.favorites, "张"],
                [
                  "downloads",
                  "我的下载",
                  ArrowDownToLine,
                  user.stats.downloads,
                  "张",
                ],
                [
                  "subscriptions",
                  "我的订阅",
                  Bookmark,
                  user.stats.subscriptions,
                  "个订阅",
                ],
                ["history", "浏览历史", Clock3, user.stats.history, "条记录"],
              ].map(([key, title, Icon, value, unit], i) => {
                const I = Icon as typeof Heart;
                return (
                  <Link
                    className={"quick-stat quick-" + i}
                    key={key as string}
                    to={"/me/" + key}
                  >
                    <span>
                      <I size={24} fill={i === 0 ? "currentColor" : "none"} />
                    </span>
                    <div>
                      <strong>{title as string}</strong>
                      <small>
                        {number(value as number)} {unit as string}
                      </small>
                    </div>
                  </Link>
                );
              })}
            </div>
            <section className="profile-section">
              <SectionTitle title="我的订阅" more="/me/subscriptions" />
              <SubscriptionList compact />
            </section>
            <section className="profile-section">
              <SectionTitle title="最近下载" more="/me/downloads" />
              <RecentDownloads />
            </section>
          </>
        )}
        {["favorites", "downloads", "history"].includes(tab) && (
          <>
            <SectionTitle
              title={tabs.find((t) => t[0] === tab)?.[1] || ""}
              subtitle={
                tab === "history"
                  ? "保留最近浏览的 50 张壁纸"
                  : tab === "downloads"
                    ? "这里记录已响应的下载请求"
                    : "每一份喜欢，都值得珍藏"
              }
            />
            <WallpaperGrid
              filters={{ scope: tab }}
              emptyText={
                tab === "favorites"
                  ? "看到喜欢的壁纸，点一下爱心收藏吧。"
                  : tab === "downloads"
                    ? "还没有下载记录，去发现喜欢的壁纸吧。"
                    : "打开壁纸详情后，会在这里留下记录。"
              }
            />
          </>
        )}
        {tab === "subscriptions" && (
          <>
            <SectionTitle
              title="我的订阅"
              subtitle="管理你喜欢的作者、主题与渠道"
            />
            <SubscriptionList />
          </>
        )}
        {tab === "settings" && (
          <SettingsPanel onEdit={() => setEditing(true)} />
        )}
      </div>
      {editing && (
        <ProfileEditor close={() => setEditing(false)} profile={user.profile} />
      )}
    </div>
  );
}
function SubscriptionList({ compact = false }: { compact?: boolean }) {
  const q = useQuery({
    queryKey: ["subscriptions"],
    queryFn: () => api<Page<Entity>>("/entities?subscribed=true"),
  });
  if (q.isLoading) return <Loading count={3} />;
  if (q.error) return <ErrorState error={q.error} retry={() => q.refetch()} />;
  const items = compact ? q.data?.items.slice(0, 4) : q.data?.items;
  return items?.length ? (
    <div className={"subscription-list " + (compact ? "compact" : "")}>
      {items.map((e) => (
        <div className="subscription-item" key={e.id}>
          <Link
            to={
              (e.kind === "author"
                ? "/authors/"
                : e.kind === "topic"
                  ? "/topics/"
                  : "/channels/") + e.id
            }
          >
            <Avatar entity={{ ...e, avatar: e.avatar || e.cover }} />
            <span>
              <strong>{e.name}</strong>
              <small>
                {e.kind === "author"
                  ? "创作者"
                  : e.kind === "topic"
                    ? "主题"
                    : "渠道"}{" "}
                · {number(e.count)} 张作品
              </small>
            </span>
          </Link>
          <Subscribe entity={e} />
        </div>
      ))}
    </div>
  ) : (
    <Empty
      title="还没有订阅"
      text="找到喜欢的作者和主题，点击订阅就会出现在这里。"
      action={
        <Link className="text-btn" to="/topics">
          发现感兴趣的主题 <ChevronRight size={15} />
        </Link>
      }
    />
  );
}
function RecentDownloads() {
  const q = useQuery({
    queryKey: ["recent-downloads"],
    queryFn: () => api<Page<Wallpaper>>("/wallpapers?scope=downloads&limit=4"),
  });
  if (q.error) return <ErrorState error={q.error} retry={() => q.refetch()} />;
  return q.data?.items.length ? (
    <div className="recent-grid">
      {q.data.items.map((w) => (
        <Link to={"/wallpaper/" + w.id} key={w.id}>
          <SmartImage src={w.images[0]?.thumbnail} alt={w.title} />
          <span>{w.title}</span>
        </Link>
      ))}
    </div>
  ) : (
    <Empty
      title="收藏美好，也留在身边"
      text="下载喜欢的壁纸后，会在这里留下足迹。"
      action={
        <Link className="secondary" to="/discover">
          去发现壁纸
        </Link>
      }
    />
  );
}
function ProfileEditor({
  close,
  profile,
}: {
  close: () => void;
  profile: Profile;
}) {
  const { refresh, notify } = useApp();
  const [value, setValue] = useState<Profile>({
      name: profile.name || "",
      bio: profile.bio || "",
      avatar: profile.avatar || "",
      cover: profile.cover || "",
    }),
    [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  const q = useQuery({
    queryKey: ["profile-covers"],
    queryFn: () => api<Page<Wallpaper>>("/wallpapers?limit=12"),
  });
  return (
    <div className="modal-backdrop" onClick={close}>
      <section
        className="modal profile-editor"
        role="dialog"
        aria-modal="true"
        aria-label="编辑资料"
        onClick={(e) => e.stopPropagation()}
      >
        <button
          className="icon-btn modal-close"
          aria-label="关闭"
          onClick={close}
        >
          <X />
        </button>
        <h2>编辑资料</h2>
        <p className="muted">让你的主页，更像你自己</p>
        <form
          onSubmit={async (e) => {
            e.preventDefault();
            setBusy(true);
            setError("");
            try {
              await api("/me/profile", "PATCH", value);
              await refresh();
              close();
              notify("资料已更新");
            } catch (e) {
              setError((e as Error).message);
            } finally {
              setBusy(false);
            }
          }}
        >
          <label>
            昵称
            <input
              value={value.name}
              onChange={(e) => setValue({ ...value, name: e.target.value })}
              maxLength={40}
              required
            />
          </label>
          <label>
            个人简介
            <textarea
              value={value.bio}
              onChange={(e) => setValue({ ...value, bio: e.target.value })}
              maxLength={200}
              rows={2}
              placeholder="介绍一下你喜欢的世界"
            />
          </label>
          <label>选择头像</label>
          <div className="avatar-picker">
            {[
              "",
              "/assets/avatar.jpg",
              "/assets/portrait.jpg",
              "/assets/cat.jpg",
              "/assets/space.jpg",
            ].map((src) => (
              <button
                type="button"
                key={src}
                aria-label={"选择头像 " + (src || "默认")}
                className={value.avatar === src ? "selected" : ""}
                onClick={() => setValue({ ...value, avatar: src })}
              >
                <Avatar entity={{ name: value.name, avatar: src }} />
              </button>
            ))}
          </div>
          <label>选择主页背景</label>
          <div className="cover-picker">
            <button
              type="button"
              className={!value.cover ? "selected" : ""}
              onClick={() => setValue({ ...value, cover: "" })}
            >
              <SmartImage src="/assets/mountain.jpg" alt="默认背景" />
            </button>
            {q.data?.items.map((w) => (
              <button
                key={w.id}
                type="button"
                className={value.cover === w.images[0]?.url ? "selected" : ""}
                onClick={() => setValue({ ...value, cover: w.images[0].url })}
              >
                <SmartImage src={w.images[0]?.thumbnail} alt={w.title} />
              </button>
            ))}
          </div>
          {error && <p className="form-error">{error}</p>}
          <button disabled={busy} className="primary full">
            {busy ? "保存中…" : "保存修改"}
          </button>
        </form>
      </section>
    </div>
  );
}
function SettingsPanel({ onEdit }: { onEdit: () => void }) {
  const { session, refresh, notify } = useApp();
  const [error, setError] = useState(""),
    [busy, setBusy] = useState(false);
  return (
    <>
      <SectionTitle title="账号设置" />
      <section className="settings-panel">
        <div className="settings-profile">
          <Avatar entity={session?.user?.profile} size="large" />
          <div>
            <h3>{session?.user?.username}</h3>
            <span className="muted">
              {session?.user?.role === "admin" ? "管理员" : "星球居民"}
            </span>
          </div>
          <button className="secondary" onClick={onEdit}>
            编辑资料
          </button>
        </div>
        <hr />
        <h3>修改密码</h3>
        <p className="muted">修改后将退出所有设备，需要重新登录。</p>
        <form
          className="password-form"
          onSubmit={async (e) => {
            e.preventDefault();
            setBusy(true);
            setError("");
            const data = new FormData(e.currentTarget);
            if (data.get("password") !== data.get("confirm")) {
              setError("两次新密码输入不一致");
              setBusy(false);
              return;
            }
            try {
              await api("/me/password", "PUT", {
                current: data.get("current"),
                password: data.get("password"),
              });
              await refresh();
              notify("密码已修改，请重新登录");
            } catch (e) {
              setError((e as Error).message);
            } finally {
              setBusy(false);
            }
          }}
        >
          <label>
            当前密码
            <input
              name="current"
              type="password"
              required
              autoComplete="current-password"
            />
          </label>
          <label>
            新密码
            <input
              name="password"
              type="password"
              minLength={10}
              maxLength={128}
              required
              autoComplete="new-password"
            />
          </label>
          <label>
            确认新密码
            <input
              name="confirm"
              type="password"
              minLength={10}
              maxLength={128}
              required
              autoComplete="new-password"
            />
          </label>
          {error && <p className="form-error">{error}</p>}
          <button className="primary" disabled={busy}>
            更新密码
          </button>
        </form>
      </section>
    </>
  );
}

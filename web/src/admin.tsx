import { useState, type FormEvent, type ReactNode } from "react";
import { NavLink, useParams } from "react-router-dom";
import {
  useInfiniteQuery,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import {
  Activity,
  KeyRound,
  Radio,
  Images,
  Users,
  Layers,
  LayoutTemplate,
  Play,
  Plus,
  RefreshCw,
  Save,
  ShieldCheck,
  X,
  ExternalLink,
} from "lucide-react";
import { api, params, type Entity, type Page, type Wallpaper } from "./api";
import { useApp } from "./context";
import {
  Empty,
  ErrorState,
  Loading,
  SmartImage,
  SectionTitle,
} from "./components";
type Source = {
  id: string;
  name: string;
  kind: string;
  query: string;
  enabled: boolean;
  interval_hours: number;
  max_items: number;
  min_short: number;
  min_long: number;
  topic_ids: string[];
  tags: string[];
  watermark?: string;
  next_run?: string;
};
type Job = {
  id: string;
  source_name: string;
  actor: string;
  status: string;
  run_id?: string;
  dataset_id?: string;
  imported: number;
  failed_items: number;
  dataset_offset: number;
  error: string;
  created_at: string;
  started_at?: string;
};
const nav = [
  ["", "运行概览", Activity],
  ["settings", "Apify 配置", KeyRound],
  ["sources", "采集来源", Radio],
  ["jobs", "任务记录", RefreshCw],
  ["content", "作品管理", Images],
  ["entities", "作者与主题", Layers],
  ["homepage", "首页配置", LayoutTemplate],
  ["users", "用户管理", Users],
] as const;
const statusNames: Record<string, string> = {
  queued: "等待采集",
  starting: "正在启动",
  uncertain: "等待核对",
  running: "采集中",
  importing: "导入中",
  retrying: "重试中",
  succeeded: "已完成",
  partial: "部分失败",
  failed: "失败",
};
function useSave() {
  const client = useQueryClient(),
    { notify } = useApp();
  const [busy, setBusy] = useState(false);
  return {
    busy,
    save: async (path: string, method: string, body?: unknown) => {
      setBusy(true);
      try {
        const result = await api(path, method, body);
        await client.invalidateQueries();
        notify("操作成功");
        return result;
      } finally {
        setBusy(false);
      }
    },
  };
}
function AdminFrame({ children }: { children: ReactNode }) {
  return (
    <div className="admin-layout">
      <aside className="admin-nav">
        <div className="admin-brand">
          <ShieldCheck size={24} />
          <span>
            星球管理中心<small>WALLPLANET CONSOLE</small>
          </span>
        </div>
        {nav.map(([key, label, Icon]) => (
          <NavLink key={key} end to={"/admin" + (key ? "/" + key : "")}>
            <Icon size={18} />
            {label}
          </NavLink>
        ))}
      </aside>
      <div className="admin-main">{children}</div>
    </div>
  );
}
export function AdminPage() {
  const { tab = "" } = useParams();
  const { session, loading, login } = useApp();
  if (loading) return <Loading />;
  if (!session?.user)
    return (
      <Empty
        title="管理员登录"
        text="使用管理员账号管理星球内容与采集任务。"
        action={
          <button className="primary" onClick={login}>
            登录
          </button>
        }
      />
    );
  if (session.user.role !== "admin")
    return <Empty title="需要管理员权限" text="当前账号无法访问管理中心。" />;
  return (
    <AdminFrame>
      {session.demo && (
        <div className="admin-notice">
          当前为独立演示环境，数据与正式站点隔离，不能启动真实采集。
        </div>
      )}
      {tab === "" ? (
        <Overview />
      ) : tab === "settings" ? (
        <Settings />
      ) : tab === "sources" ? (
        <Sources />
      ) : tab === "jobs" ? (
        <Jobs />
      ) : tab === "content" ? (
        <Content />
      ) : tab === "entities" ? (
        <Entities />
      ) : tab === "homepage" ? (
        <Homepage />
      ) : tab === "users" ? (
        <UsersPanel />
      ) : (
        <Empty title="页面不存在" />
      )}
    </AdminFrame>
  );
}
function Overview() {
  const q = useQuery({
    queryKey: ["admin", "overview"],
    queryFn: () => api<Record<string, number>>("/admin/overview"),
    refetchInterval: 15000,
  });
  return (
    <>
      <SectionTitle title="运行概览" subtitle="查看图库、用户与采集运行状态" />
      {q.error ? (
        <ErrorState error={q.error} retry={() => q.refetch()} />
      ) : (
        <div className="admin-stats">
          {[
            ["wallpapers", "图库作品"],
            ["users", "注册用户"],
            ["sources", "采集来源"],
            ["activeJobs", "进行中任务"],
            ["failedItems", "待重试条目"],
          ].map(([key, label]) => (
            <div key={key}>
              <span>{label}</span>
              <strong>{q.data?.[key] ?? "—"}</strong>
            </div>
          ))}
        </div>
      )}
      <section className="admin-panel">
        <h3>开始维护你的星球</h3>
        <ol className="setup-steps">
          <li>
            <span>01</span>
            <div>
              <h4>配置 Apify</h4>
              <p>保存 Token，采集任务由服务端统一执行。</p>
              <NavLink to="/admin/settings">前往配置 →</NavLink>
            </div>
          </li>
          <li>
            <span>02</span>
            <div>
              <h4>添加采集来源</h4>
              <p>填写 X 账号或关键词，选择主题与采集频率。</p>
              <NavLink to="/admin/sources">管理来源 →</NavLink>
            </div>
          </li>
          <li>
            <span>03</span>
            <div>
              <h4>管理内容与首页</h4>
              <p>查看已导入作品，维护标题、标签、推荐与横幅。</p>
              <NavLink to="/admin/content">查看作品 →</NavLink>
            </div>
          </li>
        </ol>
      </section>
    </>
  );
}
function Settings() {
  const q = useQuery({
    queryKey: ["admin", "settings"],
    queryFn: () =>
      api<{
        tokenConfigured: boolean;
        tokenMask: string;
        actor: string;
        updatedAt?: string;
        demo: boolean;
      }>("/admin/settings"),
  });
  const { save, busy } = useSave();
  const [token, setToken] = useState(""),
    [error, setError] = useState("");
  return (
    <>
      <SectionTitle
        title="Apify 配置"
        subtitle="密钥加密保存在服务端，仅管理员可更新"
      />
      {q.error && <ErrorState error={q.error} retry={() => q.refetch()} />}
      <section className="admin-panel narrow">
        <div className="config-status">
          <span
            className={
              "status " + (q.data?.tokenConfigured ? "succeeded" : "queued")
            }
          >
            {q.data?.tokenConfigured ? "已配置" : "未配置"}
          </span>
          <code>{q.data?.tokenMask}</code>
        </div>
        <form
          onSubmit={async (e) => {
            e.preventDefault();
            setError("");
            try {
              await save("/admin/settings", "PUT", { token });
              setToken("");
            } catch (e) {
              setError((e as Error).message);
            }
          }}
        >
          <label>
            Apify API Token
            <input
              type="password"
              value={token}
              onChange={(e) => setToken(e.target.value)}
              placeholder={
                q.data?.tokenConfigured
                  ? "输入新 Token 以替换"
                  : "输入你的 Apify Token"
              }
              autoComplete="off"
              minLength={10}
              required
            />
          </label>
          <label>
            当前 Actor
            <input readOnly value="xquik/x-tweet-scraper" />
          </label>
          <p className="muted small">
            全局并发 1，运行间隔至少 5 分钟。未启用来源时不会自动采集。
          </p>
          {error && <p className="form-error">{error}</p>}
          <button className="primary" disabled={busy || q.data?.demo}>
            <Save size={16} />
            保存配置
          </button>
        </form>
      </section>
    </>
  );
}
function Sources() {
  const q = useQuery({
    queryKey: ["admin", "sources"],
    queryFn: () => api<Page<Source>>("/admin/sources"),
  });
  const [edit, setEdit] = useState<Source | null>(null);
  const [runSource, setRunSource] = useState<Source | null>(null);
  const [runError, setRunError] = useState("");
  const { save, busy } = useSave();
  return (
    <>
      <SectionTitle
        title="采集来源"
        subtitle="订阅对象由平台维护，用户无需配置抓取器"
      >
        <button
          className="primary"
          onClick={() =>
            setEdit({
              id: "",
              name: "",
              kind: "accounts",
              query: "",
              enabled: false,
              interval_hours: 168,
              max_items: 100,
              min_short: 720,
              min_long: 1280,
              topic_ids: [],
              tags: [],
            })
          }
        >
          <Plus size={16} />
          添加来源
        </button>
      </SectionTitle>
      <div className="admin-notice">
        Xquik 按交付结果收费：$0.15 / 1000 条，另计 Apify
        平台用量。没有单独启动费、查询费或最低 50 条收费。
        <a
          href="https://apify.com/xquik/x-tweet-scraper/pricing"
          target="_blank"
          rel="noreferrer"
        >
          {" "}
          查看 Actor 计费规则 ↗
        </a>
        <br />
        建议先手动采集，避免多个低产出账号频繁运行。浏览本站、下载已入库图片和重试本地导入不会重新启动
        Actor。
      </div>
      {q.error ? (
        <ErrorState error={q.error} retry={() => q.refetch()} />
      ) : q.data?.items.length ? (
        <div className="admin-table-wrap">
          <table>
            <thead>
              <tr>
                <th>来源</th>
                <th>查询</th>
                <th>频率 / 条数</th>
                <th>状态</th>
                <th>上次成功水位</th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody>
              {q.data.items.map((s) => (
                <tr key={s.id}>
                  <td>
                    <strong>{s.name}</strong>
                    <small>
                      {s.kind === "accounts"
                        ? "X 作者组"
                        : s.kind === "account"
                          ? "旧单账号（请合并）"
                          : "关键词"}
                    </small>
                  </td>
                  <td>
                    <code>{s.query}</code>
                  </td>
                  <td>
                    每 {s.interval_hours} 小时 / {s.max_items}
                  </td>
                  <td>
                    <span
                      className={
                        "status " + (s.enabled ? "succeeded" : "queued")
                      }
                    >
                      {s.enabled ? "已启用" : "已暂停"}
                    </span>
                  </td>
                  <td>
                    {s.watermark
                      ? new Date(s.watermark).toLocaleString("zh-CN")
                      : "尚未采集"}
                  </td>
                  <td>
                    <div className="table-actions">
                      <button className="text-btn" onClick={() => setEdit(s)}>
                        编辑
                      </button>
                      <button
                        className="text-btn"
                        disabled={s.kind === "account"}
                        onClick={() => {
                          setRunError("");
                          setRunSource(s);
                        }}
                      >
                        <Play size={13} />
                        采集（付费）
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : (
        <Empty
          title="添加第一个采集来源"
          text="输入公开 X 账号或关键词，开始汇集壁纸。"
        />
      )}
      {edit && <SourceEditor source={edit} close={() => setEdit(null)} />}
      {runSource && (
        <Editor
          title="确认付费采集"
          close={() => {
            if (!busy) setRunSource(null);
          }}
        >
          <p>来源：{runSource.name}</p>
          <p>
            整批最多交付 {runSource.max_items} 条，共用一个 Actor
            运行。结果费估算至多 ${(runSource.max_items * 0.00015).toFixed(4)}
            ，另计平台用量，实际以 Apify 账单为准。
          </p>
          <p className="muted">
            没有结果或图片未通过清晰度筛选，也可能产生费用。此操作会创建新的
            Actor 运行。
          </p>
          {runError && (
            <p className="form-error" role="alert">
              {runError}
            </p>
          )}
          <div className="table-actions">
            <button
              className="secondary"
              disabled={busy}
              onClick={() => setRunSource(null)}
            >
              取消
            </button>
            <button
              className="primary"
              disabled={busy}
              onClick={async () => {
                try {
                  await save("/admin/sources/" + runSource.id + "/run", "POST");
                  setRunSource(null);
                } catch (e) {
                  setRunError((e as Error).message);
                }
              }}
            >
              确认采集
            </button>
          </div>
        </Editor>
      )}
    </>
  );
}
function SourceEditor({
  source,
  close,
}: {
  source: Source;
  close: () => void;
}) {
  const [value, setValue] = useState({
      ...source,
      kind: source.kind === "account" ? "accounts" : source.kind,
    }),
    [error, setError] = useState("");
  const [preview, setPreview] = useState("");
  const [previewBusy, setPreviewBusy] = useState(false);
  const [quality, setQuality] = useState(
    source.min_short === 720 && source.min_long === 1280
      ? "standard"
      : source.min_short === 1080 && source.min_long === 1920
        ? "high"
        : source.min_short === 1 && source.min_long === 1
          ? "all"
          : "custom",
  );
  const { save, busy } = useSave();
  const topics = useQuery({
    queryKey: ["entities", "topic"],
    queryFn: () => api<Page<Entity>>("/entities?kind=topic"),
  });
  const patch = (key: keyof Source, v: unknown) => {
    setPreview("");
    setValue((x) => ({ ...x, [key]: v }));
  };
  return (
    <Editor title={value.id ? "编辑来源" : "添加采集来源"} close={close}>
      <form
        onSubmit={async (e) => {
          e.preventDefault();
          setError("");
          try {
            await save(
              "/admin/sources" + (value.id ? "/" + value.id : ""),
              value.id ? "PUT" : "POST",
              value,
            );
            close();
          } catch (e) {
            setError((e as Error).message);
          }
        }}
      >
        <label>
          来源名称
          <input
            value={value.name}
            onChange={(e) => patch("name", e.target.value)}
            required
            maxLength={100}
          />
        </label>
        <div className="form-row">
          <label>
            来源类型
            <select
              value={value.kind}
              onChange={(e) => patch("kind", e.target.value)}
            >
              <option value="accounts">X 作者组（批量）</option>
              <option value="keyword">关键词 / 高级搜索</option>
            </select>
          </label>
          <label>
            {value.kind === "accounts"
              ? "作者列表（2–100 个用户名）"
              : "查询关键词"}
            <textarea
              value={value.query}
              onChange={(e) => patch("query", e.target.value)}
              placeholder={
                value.kind === "accounts"
                  ? "例如 NASA, NASAHubble, NASAWebb"
                  : "例如 landscape wallpaper"
              }
              required
              maxLength={5000}
            />
          </label>
        </div>
        <p className="muted small">
          作者用逗号或换行分隔，每 20 位合为一条 OR
          查询。整组只启动一次，条数上限由所有查询共享。
        </p>
        <button
          type="button"
          className="secondary"
          disabled={previewBusy}
          onClick={async () => {
            setPreviewBusy(true);
            setError("");
            try {
              const result = await api<{ input: unknown }>(
                "/admin/sources/preview",
                "POST",
                value,
              );
              setPreview(JSON.stringify(result.input, null, 2));
            } catch (e) {
              setError((e as Error).message);
            } finally {
              setPreviewBusy(false);
            }
          }}
        >
          预览批量查询（免费）
        </button>
        {preview && (
          <pre
            style={{
              whiteSpace: "pre-wrap",
              overflowWrap: "anywhere",
              maxHeight: 240,
              overflow: "auto",
            }}
          >
            {preview}
          </pre>
        )}
        <div className="form-row">
          <label>
            间隔（小时）
            <input
              type="number"
              min={1}
              max={720}
              value={value.interval_hours}
              onChange={(e) => patch("interval_hours", +e.target.value)}
            />
          </label>
          <label>
            最多条数
            <input
              type="number"
              min={1}
              max={1000}
              value={value.max_items}
              onChange={(e) => patch("max_items", +e.target.value)}
            />
          </label>
        </div>
        <label>
          图片清晰度
          <select
            value={quality}
            onChange={(e) => {
              const choice = e.target.value;
              setQuality(choice);
              const sizes: Record<string, [number, number]> = {
                standard: [720, 1280],
                high: [1080, 1920],
                all: [1, 1],
              };
              if (sizes[choice])
                setValue((v) => ({
                  ...v,
                  min_short: sizes[choice][0],
                  min_long: sizes[choice][1],
                }));
            }}
          >
            <option value="standard">标准（推荐）</option>
            <option value="high">高清</option>
            <option value="all">不限尺寸</option>
            <option value="custom">自定义尺寸筛选</option>
          </select>
        </label>
        <p className="muted small">
          {quality === "standard"
            ? "自动保留至少 1280 × 720 的图片，横图竖图都可以；一般不需要调整。"
            : quality === "high"
              ? "自动保留至少 1920 × 1080 的图片，横图竖图都可以。"
              : quality === "all"
                ? "保留所有支持的静态图片，小图也会入库。"
                : "按图片较短的一边和较长的一边筛选，横竖方向不影响结果。"}
          <br />
          这是抓取后的入库筛选，不是要求来源按此尺寸发图；不会放大原图，也不会减少
          Apify 已产生的费用。
        </p>
        {quality === "custom" && (
          <div className="form-row">
            <label>
              最小短边（像素）
              <input
                type="number"
                min={1}
                value={value.min_short}
                onChange={(e) => patch("min_short", +e.target.value)}
              />
            </label>
            <label>
              最小长边（像素）
              <input
                type="number"
                min={1}
                value={value.min_long}
                onChange={(e) => patch("min_long", +e.target.value)}
              />
            </label>
          </div>
        )}
        <label>归属主题</label>
        <div className="checkbox-group">
          {topics.data?.items.map((t) => (
            <label key={t.id}>
              <input
                type="checkbox"
                checked={value.topic_ids.includes(t.id)}
                onChange={(e) =>
                  patch(
                    "topic_ids",
                    e.target.checked
                      ? [...value.topic_ids, t.id]
                      : value.topic_ids.filter((id) => id !== t.id),
                  )
                }
              />
              {t.name}
            </label>
          ))}
        </div>
        <label>
          附加标签（逗号分隔）
          <input
            value={value.tags.join(",")}
            onChange={(e) =>
              patch("tags", e.target.value.split(/[,，]/).filter(Boolean))
            }
          />
        </label>
        <label className="checkbox">
          <input
            type="checkbox"
            checked={value.enabled}
            onChange={(e) => patch("enabled", e.target.checked)}
          />
          启用定时采集并自动发布
        </label>
        {value.enabled && (
          <p className="form-error">
            启用后将持续产生费用。每 {value.interval_hours}{" "}
            小时一次、每次满额估算，30 天结果费约 $
            {(
              (720 / Math.max(1, value.interval_hours)) *
              value.max_items *
              0.00015
            ).toFixed(2)}{" "}
            / 来源，另计平台用量。
          </p>
        )}
        {error && <p className="form-error">{error}</p>}
        <button className="primary full" disabled={busy}>
          保存来源
        </button>
      </form>
    </Editor>
  );
}
function Editor({
  title,
  close,
  children,
}: {
  title: string;
  close: () => void;
  children: ReactNode;
}) {
  return (
    <div className="modal-backdrop" onClick={close}>
      <section
        className="modal admin-editor"
        role="dialog"
        aria-modal="true"
        aria-label={title}
        onClick={(e) => e.stopPropagation()}
      >
        <button
          className="icon-btn modal-close"
          aria-label="关闭"
          onClick={close}
        >
          <X />
        </button>
        <h2>{title}</h2>
        {children}
      </section>
    </div>
  );
}
function Jobs() {
  const q = useInfiniteQuery({
    queryKey: ["admin", "jobs"],
    initialPageParam: "",
    queryFn: ({ pageParam }) =>
      api<Page<Job>>("/admin/jobs?" + params({ cursor: pageParam })),
    getNextPageParam: (p) => p.nextCursor || undefined,
    refetchInterval: 15000,
  });
  const { save } = useSave(),
    { notify } = useApp();
  const [selected, setSelected] = useState<Job | null>(null),
    [runId, setRunId] = useState("");
  const failures = useQuery({
    queryKey: ["admin", "job-items", selected?.id],
    queryFn: () =>
      api<
        Page<{
          id: string;
          source_key: string;
          error: string;
          attempts: number;
          resolved: boolean;
        }>
      >("/admin/jobs/" + selected!.id + "/items"),
    enabled: !!selected,
  });
  return (
    <>
      <SectionTitle
        title="任务记录"
        subtitle="每 15 秒更新 · 中断后的任务自动恢复"
      >
        <button className="secondary" onClick={() => q.refetch()}>
          <RefreshCw size={15} />
          刷新
        </button>
      </SectionTitle>
      {q.error ? (
        <ErrorState error={q.error} retry={() => q.refetch()} />
      ) : (
        <div className="admin-table-wrap">
          <table>
            <thead>
              <tr>
                <th>来源 / 时间</th>
                <th>状态</th>
                <th>导入 / 处理 / 失败</th>
                <th>运行信息</th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody>
              {q.data?.pages
                .flatMap((p) => p.items)
                .map((j) => (
                  <tr key={j.id}>
                    <td>
                      <strong>
                        {j.source_name}
                        <small>{j.actor}</small>
                      </strong>
                      <small>
                        {new Date(j.created_at).toLocaleString("zh-CN")}
                      </small>
                    </td>
                    <td>
                      <span className={"status " + j.status}>
                        {statusNames[j.status] || j.status}
                      </span>
                    </td>
                    <td>
                      {j.imported} / {j.dataset_offset} / {j.failed_items}
                    </td>
                    <td>
                      {j.run_id && (
                        <a
                          className="text-btn"
                          href={
                            "https://console.apify.com/actors/runs/" + j.run_id
                          }
                          target="_blank"
                          rel="noreferrer"
                        >
                          {j.run_id}
                          <ExternalLink size={12} />
                        </a>
                      )}
                      <small className="form-error">{j.error}</small>
                    </td>
                    <td>
                      <div className="table-actions">
                        <button
                          className="text-btn"
                          onClick={() => {
                            setSelected(j);
                            setRunId("");
                          }}
                        >
                          {j.status === "uncertain" ? "核对运行" : "详情"}
                        </button>
                        {["partial", "failed"].includes(j.status) &&
                          j.dataset_id && (
                            <button
                              className="text-btn"
                              onClick={() =>
                                save(
                                  "/admin/jobs/" + j.id + "/retry",
                                  "POST",
                                ).catch((e) => notify(e.message))
                              }
                            >
                              重试失败项
                            </button>
                          )}
                      </div>
                    </td>
                  </tr>
                ))}
            </tbody>
          </table>
          {q.data?.pages[0].items.length === 0 && (
            <Empty title="暂无采集任务" text="配置来源后，任务会在这里显示。" />
          )}
        </div>
      )}
      {q.hasNextPage && (
        <button className="secondary" onClick={() => q.fetchNextPage()}>
          加载更多
        </button>
      )}
      {selected && (
        <Editor title="任务详情" close={() => setSelected(null)}>
          <p className="muted">
            {selected.source_name} · {statusNames[selected.status]}
          </p>
          {selected.status === "uncertain" && (
            <section className="reconcile-box">
              <p>
                请核对 Apify
                控制台中的运行时间和输入，关联已创建的运行；确认没有运行后才能结束此任务。
              </p>
              <label>
                已有 Run ID
                <input
                  value={runId}
                  onChange={(e) => setRunId(e.target.value)}
                />
              </label>
              <div className="table-actions">
                <button
                  className="primary"
                  disabled={!runId}
                  onClick={() =>
                    save("/admin/jobs/" + selected.id + "/reconcile", "POST", {
                      runId,
                    })
                      .then(() => setSelected(null))
                      .catch((e) => notify(e.message))
                  }
                >
                  验证并关联
                </button>
                <button
                  className="secondary"
                  onClick={() =>
                    save("/admin/jobs/" + selected.id + "/reconcile", "POST", {
                      confirmAbsent: true,
                    })
                      .then(() => setSelected(null))
                      .catch((e) => notify(e.message))
                  }
                >
                  已核对，确实未创建
                </button>
              </div>
            </section>
          )}
          {failures.error && (
            <p className="form-error">{failures.error.message}</p>
          )}
          {failures.data?.items.map((i) => (
            <div className="job-item" key={i.id}>
              <strong>{i.source_key}</strong>
              <span
                className={"status " + (i.resolved ? "succeeded" : "failed")}
              >
                {i.resolved ? "已处理" : "待重试"} · {i.attempts} 次
              </span>
              {i.error && <p>{i.error}</p>}
              {!i.resolved &&
                ["partial", "failed"].includes(selected.status) && (
                  <button
                    className="text-btn"
                    onClick={() =>
                      save(
                        "/admin/jobs/" +
                          selected.id +
                          "/items/" +
                          i.id +
                          "/retry",
                        "POST",
                      )
                        .then(() => setSelected(null))
                        .catch((e) => notify(e.message))
                    }
                  >
                    仅重试此条目
                  </button>
                )}
            </div>
          ))}
          {failures.data?.items.length === 0 && (
            <p className="muted">尚无导入条目。</p>
          )}
        </Editor>
      )}
    </>
  );
}
function Content() {
  const [search, setSearch] = useState(""),
    [editing, setEditing] = useState<Wallpaper | null>(null);
  const q = useInfiniteQuery({
    queryKey: ["admin", "content", search],
    initialPageParam: "",
    queryFn: ({ pageParam }) =>
      api<Page<Wallpaper>>(
        "/admin/wallpapers?" + params({ q: search, cursor: pageParam }),
      ),
    getNextPageParam: (p) => p.nextCursor || undefined,
  });
  return (
    <>
      <SectionTitle title="作品管理" subtitle="维护作品信息、标签与发布状态" />
      <input
        className="admin-search"
        placeholder="搜索作品标题"
        value={search}
        onChange={(e) => setSearch(e.target.value)}
      />
      {q.error ? (
        <ErrorState error={q.error} retry={() => q.refetch()} />
      ) : (
        <div className="admin-table-wrap">
          <table>
            <thead>
              <tr>
                <th>作品</th>
                <th>作者</th>
                <th>标签</th>
                <th>状态</th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody>
              {q.data?.pages
                .flatMap((p) => p.items)
                .map((w) => (
                  <tr key={w.id}>
                    <td>
                      <div className="admin-work">
                        <SmartImage src={w.images[0]?.thumbnail} alt="" />
                        <span>
                          <strong>{w.title}</strong>
                          <small>
                            {w.images.length} 张图片{" "}
                            {w.featured ? "· 已推荐" : ""}
                          </small>
                        </span>
                      </div>
                    </td>
                    <td>{w.author.name}</td>
                    <td>{w.tags.join(" · ")}</td>
                    <td>
                      <span
                        className={
                          "status " +
                          (w.status === "published" ? "succeeded" : "queued")
                        }
                      >
                        {w.status === "published" ? "已发布" : "已下架"}
                      </span>
                    </td>
                    <td>
                      <button
                        className="text-btn"
                        onClick={() => setEditing(w)}
                      >
                        编辑
                      </button>
                    </td>
                  </tr>
                ))}
            </tbody>
          </table>
          {q.data?.pages[0].items.length === 0 && (
            <Empty title="暂无作品" text="采集到合格图片后，会自动入库。" />
          )}
        </div>
      )}
      {q.hasNextPage && (
        <button className="secondary" onClick={() => q.fetchNextPage()}>
          加载更多
        </button>
      )}
      {editing && <ContentEditor w={editing} close={() => setEditing(null)} />}
    </>
  );
}
function ContentEditor({ w, close }: { w: Wallpaper; close: () => void }) {
  const [v, setV] = useState(w),
    [error, setError] = useState("");
  const { save, busy } = useSave();
  const topics = useQuery({
    queryKey: ["entities", "topic"],
    queryFn: () => api<Page<Entity>>("/entities?kind=topic"),
  });
  return (
    <Editor title="编辑作品" close={close}>
      <form
        onSubmit={async (e) => {
          e.preventDefault();
          try {
            await save("/admin/wallpapers/" + w.id, "PATCH", v);
            close();
          } catch (e) {
            setError((e as Error).message);
          }
        }}
      >
        <label>
          标题
          <input
            value={v.title}
            required
            onChange={(e) => setV({ ...v, title: e.target.value })}
          />
        </label>
        <label>
          描述
          <textarea
            rows={3}
            value={v.description}
            onChange={(e) => setV({ ...v, description: e.target.value })}
          />
        </label>
        <label>
          标签（逗号分隔）
          <input
            value={v.tags.join(",")}
            onChange={(e) =>
              setV({
                ...v,
                tags: e.target.value.split(/[,，]/).filter(Boolean),
              })
            }
          />
        </label>
        <label>主题</label>
        <div className="checkbox-group">
          {topics.data?.items.map((t) => (
            <label key={t.id}>
              <input
                type="checkbox"
                checked={v.topicIds.includes(t.id)}
                onChange={(e) =>
                  setV({
                    ...v,
                    topicIds: e.target.checked
                      ? [...v.topicIds, t.id]
                      : v.topicIds.filter((id) => id !== t.id),
                  })
                }
              />
              {t.name}
            </label>
          ))}
        </div>
        <label>
          发布状态
          <select
            value={v.status}
            onChange={(e) => setV({ ...v, status: e.target.value })}
          >
            <option value="published">已发布</option>
            <option value="hidden">已下架</option>
          </select>
        </label>
        <label className="checkbox">
          <input
            type="checkbox"
            checked={v.featured}
            onChange={(e) => setV({ ...v, featured: e.target.checked })}
          />
          推荐作品
        </label>
        {error && <p className="form-error">{error}</p>}
        <button className="primary full" disabled={busy}>
          保存修改
        </button>
      </form>
    </Editor>
  );
}
function Entities() {
  const [kind, setKind] = useState("topic"),
    [editing, setEditing] = useState<Entity | null>(null);
  const q = useInfiniteQuery({
    queryKey: ["admin", "entities", kind],
    initialPageParam: "",
    queryFn: ({ pageParam }) =>
      api<Page<Entity>>("/entities?" + params({ kind, cursor: pageParam })),
    getNextPageParam: (p) => p.nextCursor || undefined,
  });
  return (
    <>
      <SectionTitle title="作者与主题" subtitle="配置展示信息和推荐内容">
        <button
          className="primary"
          disabled={kind === "channel"}
          onClick={() =>
            setEditing({
              id: "",
              kind: kind as Entity["kind"],
              name: "",
              description: "",
              cover: "",
              avatar: "",
              featured: false,
            })
          }
        >
          <Plus size={15} />
          添加{kind === "topic" ? "主题" : "作者"}
        </button>
      </SectionTitle>
      <div className="tabs compact">
        {[
          ["topic", "主题"],
          ["author", "作者"],
          ["channel", "渠道"],
        ].map(([k, t]) => (
          <button
            key={k}
            className={kind === k ? "active" : ""}
            onClick={() => setKind(k)}
          >
            {t}
          </button>
        ))}
      </div>
      {q.error ? (
        <ErrorState error={q.error} retry={() => q.refetch()} />
      ) : (
        <div className="admin-table-wrap">
          <table>
            <thead>
              <tr>
                <th>名称</th>
                <th>描述</th>
                <th>作品数</th>
                <th>推荐</th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody>
              {q.data?.pages
                .flatMap((p) => p.items)
                .map((e) => (
                  <tr key={e.id}>
                    <td>
                      <strong>{e.name}</strong>
                    </td>
                    <td>{e.description}</td>
                    <td>{e.count}</td>
                    <td>{e.featured ? "是" : "否"}</td>
                    <td>
                      <button
                        className="text-btn"
                        onClick={() => setEditing(e)}
                      >
                        编辑
                      </button>
                    </td>
                  </tr>
                ))}
            </tbody>
          </table>
        </div>
      )}
      {q.hasNextPage && (
        <button className="secondary" onClick={() => q.fetchNextPage()}>
          加载更多
        </button>
      )}
      {editing && (
        <EntityEditor entity={editing} close={() => setEditing(null)} />
      )}
    </>
  );
}
function EntityEditor({
  entity,
  close,
}: {
  entity: Entity;
  close: () => void;
}) {
  const [v, setV] = useState(entity),
    [error, setError] = useState("");
  const { save, busy } = useSave();
  return (
    <Editor title="编辑展示信息" close={close}>
      <form
        onSubmit={async (e) => {
          e.preventDefault();
          try {
            await save(
              "/admin/entities" + (v.id ? "/" + v.id : ""),
              v.id ? "PUT" : "POST",
              v,
            );
            close();
          } catch (e) {
            setError((e as Error).message);
          }
        }}
      >
        <label>
          名称
          <input
            value={v.name}
            required
            maxLength={100}
            onChange={(e) => setV({ ...v, name: e.target.value })}
          />
        </label>
        <label>
          描述
          <textarea
            value={v.description || ""}
            maxLength={500}
            onChange={(e) => setV({ ...v, description: e.target.value })}
          />
        </label>
        <label>
          封面地址（站内图片）
          <input
            value={v.cover || ""}
            placeholder="/media/... 或 /assets/..."
            onChange={(e) => setV({ ...v, cover: e.target.value })}
          />
        </label>
        <label>
          头像地址
          <input
            value={v.avatar || ""}
            placeholder="/media/... 或 /assets/..."
            onChange={(e) => setV({ ...v, avatar: e.target.value })}
          />
        </label>
        <label className="checkbox">
          <input
            type="checkbox"
            checked={!!v.featured}
            onChange={(e) => setV({ ...v, featured: e.target.checked })}
          />
          设为推荐
        </label>
        {error && <p className="form-error">{error}</p>}
        <button className="primary full" disabled={busy}>
          保存
        </button>
      </form>
    </Editor>
  );
}
type Banner = { title: string; subtitle: string; image: string; href: string };
function Homepage() {
  const q = useQuery({
    queryKey: ["home"],
    queryFn: () => api<{ banners: Banner[] }>("/home"),
  });
  if (q.isLoading) return <Loading count={2} />;
  if (q.error) return <ErrorState error={q.error} retry={() => q.refetch()} />;
  return <HomepageForm initial={q.data?.banners || []} />;
}
function HomepageForm({ initial }: { initial: Banner[] }) {
  const [banners, setBanners] = useState(initial),
    [error, setError] = useState("");
  const { save, busy } = useSave();
  const set = (i: number, k: keyof Banner, v: string) =>
    setBanners((b) =>
      b.map((item, j) => (i === j ? { ...item, [k]: v } : item)),
    );
  return (
    <>
      <SectionTitle
        title="首页横幅"
        subtitle="最多 6 张，支持标题换行与站内跳转"
      >
        <button
          className="secondary"
          disabled={banners.length >= 6}
          onClick={() =>
            setBanners([
              ...banners,
              { title: "", subtitle: "", image: "", href: "/discover" },
            ])
          }
        >
          <Plus size={16} />
          添加横幅
        </button>
      </SectionTitle>
      <form
        onSubmit={async (e) => {
          e.preventDefault();
          setError("");
          try {
            await save("/admin/homepage", "PUT", { banners });
          } catch (e) {
            setError((e as Error).message);
          }
        }}
      >
        {banners.map((b, i) => (
          <section className="admin-panel banner-editor" key={i}>
            <div className="banner-editor-preview">
              <SmartImage src={b.image} alt={"横幅 " + (i + 1)} />
              <button
                type="button"
                className="text-btn"
                onClick={() => setBanners(banners.filter((_, j) => j !== i))}
              >
                移除横幅
              </button>
            </div>
            <div>
              <label>
                标题
                <textarea
                  value={b.title}
                  required
                  maxLength={100}
                  onChange={(e) => set(i, "title", e.target.value)}
                />
              </label>
              <label>
                副标题
                <input
                  value={b.subtitle}
                  maxLength={200}
                  onChange={(e) => set(i, "subtitle", e.target.value)}
                />
              </label>
              <label>
                图片地址
                <input
                  value={b.image}
                  placeholder="/media/... 或 /assets/..."
                  required
                  onChange={(e) => set(i, "image", e.target.value)}
                />
              </label>
              <label>
                跳转路径
                <input
                  value={b.href}
                  placeholder="/discover?topic=nature"
                  required
                  onChange={(e) => set(i, "href", e.target.value)}
                />
              </label>
            </div>
          </section>
        ))}
        {!banners.length && (
          <Empty title="尚未设置横幅" text="添加横幅后，首页会自动开启轮播。" />
        )}
        {error && <p className="form-error">{error}</p>}
        <button className="primary" disabled={busy}>
          <Save size={16} />
          保存首页配置
        </button>
      </form>
    </>
  );
}
function UsersPanel() {
  const q = useInfiniteQuery({
    queryKey: ["admin", "users"],
    initialPageParam: "",
    queryFn: ({ pageParam }) =>
      api<
        Page<{
          id: string;
          username: string;
          role: string;
          disabled: boolean;
          created_at: string;
        }>
      >("/admin/users?" + params({ cursor: pageParam })),
    getNextPageParam: (p) => p.nextCursor || undefined,
  });
  const { save } = useSave(),
    { notify } = useApp();
  return (
    <>
      <SectionTitle
        title="用户管理"
        subtitle="管理员账号只能通过服务端命令创建"
      />
      {q.error ? (
        <ErrorState error={q.error} retry={() => q.refetch()} />
      ) : (
        <div className="admin-table-wrap">
          <table>
            <thead>
              <tr>
                <th>账号</th>
                <th>角色</th>
                <th>注册时间</th>
                <th>状态</th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody>
              {q.data?.pages
                .flatMap((p) => p.items)
                .map((u) => (
                  <tr key={u.id}>
                    <td>{u.username}</td>
                    <td>{u.role === "admin" ? "管理员" : "用户"}</td>
                    <td>
                      {new Date(u.created_at).toLocaleDateString("zh-CN")}
                    </td>
                    <td>{u.disabled ? "已停用" : "正常"}</td>
                    <td>
                      {u.role === "user" && (
                        <button
                          className="text-btn"
                          onClick={() =>
                            save("/admin/users/" + u.id, "PATCH", {
                              disabled: !u.disabled,
                            }).catch((e) => notify(e.message))
                          }
                        >
                          {u.disabled ? "启用" : "停用"}
                        </button>
                      )}
                    </td>
                  </tr>
                ))}
            </tbody>
          </table>
        </div>
      )}
      {q.hasNextPage && (
        <button className="secondary" onClick={() => q.fetchNextPage()}>
          加载更多
        </button>
      )}
    </>
  );
}

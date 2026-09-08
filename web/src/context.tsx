import { createContext, useContext, useState, type ReactNode } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { AnimatePresence, motion } from "motion/react";
import { X, Check, LoaderCircle, Orbit, Eye, EyeOff } from "lucide-react";
import { api, setCSRF, type Session } from "./api";
type AppContextValue = {
  session: Session | undefined;
  loading: boolean;
  notify: (text: string) => void;
  login: () => void;
  guard: (action: () => void | Promise<void>) => void;
  refresh: () => Promise<void>;
};
const Context = createContext<AppContextValue>(null!);
export const useApp = () => useContext(Context);
export function AppProvider({ children }: { children: ReactNode }) {
  const client = useQueryClient();
  const session = useQuery({
    queryKey: ["session"],
    queryFn: async () => {
      const s = await api<Session>("/auth/me");
      setCSRF(s.csrf);
      return s;
    },
    retry: 1,
  });
  const [auth, setAuth] = useState(false),
    [toast, setToast] = useState("");
  const notify = (text: string) => {
    setToast(text);
    window.setTimeout(() => setToast((t) => (t === text ? "" : t)), 3500);
  };
  const refresh = async () => {
    await client.invalidateQueries();
  };
  const guard = (fn: () => void | Promise<void>) => {
    if (!session.data?.user) {
      setAuth(true);
      return;
    }
    Promise.resolve(fn()).catch((e) => notify(e.message));
  };
  return (
    <Context.Provider
      value={{
        session: session.data,
        loading: session.isLoading,
        notify,
        login: () => setAuth(true),
        guard,
        refresh,
      }}
    >
      {children}
      <AnimatePresence>
        {auth && (
          <AuthModal
            close={() => setAuth(false)}
            done={async () => {
              await refresh();
              setAuth(false);
              notify("欢迎来到壁纸星球");
            }}
          />
        )}
        {toast && (
          <motion.div
            className="toast"
            role="status"
            initial={{ opacity: 0, y: 15 }}
            animate={{ opacity: 1, y: 0 }}
            exit={{ opacity: 0, y: 10 }}
          >
            <Check size={17} />
            {toast}
          </motion.div>
        )}
      </AnimatePresence>
    </Context.Provider>
  );
}
function AuthModal({
  close,
  done,
}: {
  close: () => void;
  done: () => Promise<void>;
}) {
  const [mode, setMode] = useState("login"),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false),
    [show, setShow] = useState(false);
  return (
    <div className="modal-backdrop" onClick={close}>
      <motion.section
        role="dialog"
        aria-modal="true"
        aria-labelledby="auth-title"
        className="modal auth-modal"
        initial={{ opacity: 0, scale: 0.96, y: 15 }}
        animate={{ opacity: 1, scale: 1, y: 0 }}
        exit={{ opacity: 0, scale: 0.96 }}
        onClick={(e) => e.stopPropagation()}
      >
        <button
          className="icon-btn modal-close"
          aria-label="关闭"
          onClick={close}
        >
          <X />
        </button>
        <div className="auth-orbit">
          <Orbit size={35} />
        </div>
        <h2 id="auth-title">
          {mode === "login" ? "欢迎回到壁纸星球" : "开启你的壁纸世界"}
        </h2>
        <p className="muted">收藏喜欢的风景，订阅心动的灵感</p>
        <form
          onSubmit={async (e) => {
            e.preventDefault();
            setBusy(true);
            setError("");
            const d = new FormData(e.currentTarget);
            try {
              const r = await api<{ csrf: string }>(
                "/auth/" + mode,
                "POST",
                Object.fromEntries(d),
              );
              setCSRF(r.csrf);
              await done();
            } catch (e) {
              setError((e as Error).message);
            } finally {
              setBusy(false);
            }
          }}
        >
          <label>
            账号
            <input
              autoFocus
              name="username"
              autoComplete="username"
              minLength={3}
              maxLength={32}
              required
              placeholder="请输入账号"
            />
          </label>
          <label>
            密码
            <div className="password-field">
              <input
                name="password"
                type={show ? "text" : "password"}
                autoComplete={
                  mode === "login" ? "current-password" : "new-password"
                }
                minLength={10}
                maxLength={128}
                required
                placeholder="至少 10 个字符"
              />
              <button
                type="button"
                className="icon-btn"
                aria-label="显示密码"
                onClick={() => setShow(!show)}
              >
                {show ? <EyeOff size={18} /> : <Eye size={18} />}
              </button>
            </div>
          </label>
          {error && (
            <p className="form-error" role="alert">
              {error}
            </p>
          )}
          <button className="primary full" disabled={busy}>
            {busy ? <LoaderCircle className="spin" size={18} /> : null}
            {mode === "login" ? "登录" : "创建账号"}
          </button>
        </form>
        <p className="auth-switch">
          {mode === "login" ? "还没有账号？" : "已经有账号？"}
          <button
            className="text-btn"
            onClick={() => {
              setMode(mode === "login" ? "register" : "login");
              setError("");
            }}
          >
            {mode === "login" ? "立即注册" : "去登录"}
          </button>
        </p>
      </motion.section>
    </div>
  );
}

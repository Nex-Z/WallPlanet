import { useEffect, useRef } from "react";

export function AutoLoadMore({
  hasMore,
  busy,
  error,
  load,
}: {
  hasMore: boolean;
  busy: boolean;
  error: boolean;
  load: () => Promise<unknown>;
}) {
  const sentinel = useRef<HTMLDivElement>(null);
  const pending = useRef(false);
  const supported = typeof IntersectionObserver !== "undefined";
  const next = async () => {
    if (pending.current || busy || !hasMore) return;
    pending.current = true;
    try {
      await load();
    } finally {
      pending.current = false;
    }
  };
  useEffect(() => {
    if (!supported || !hasMore || busy || error || !sentinel.current) return;
    const observer = new IntersectionObserver(
      ([entry]) => {
        if (entry.isIntersecting) void next();
      },
      { rootMargin: "500px 0px" },
    );
    observer.observe(sentinel.current);
    return () => observer.disconnect();
  }, [supported, hasMore, busy, error, load]);
  return (
    <div ref={sentinel} className="load-more" aria-live="polite">
      {busy ? (
        <span>正在加载更多壁纸…</span>
      ) : error ? (
        <button className="secondary" onClick={() => void next()}>
          加载失败，点击重试
        </button>
      ) : !hasMore ? (
        <span className="muted small">已展示全部壁纸</span>
      ) : !supported ? (
        <button className="secondary" onClick={() => void next()}>
          加载更多壁纸
        </button>
      ) : (
        <span className="muted small">向下滚动，发现更多</span>
      )}
    </div>
  );
}

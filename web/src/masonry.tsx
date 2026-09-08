import { useLayoutEffect, useRef, useState } from "react";
import type { Wallpaper } from "./api";
import { WallpaperCard } from "./components";
export function Masonry({ items }: { items: Wallpaper[] }) {
  const ref = useRef<HTMLDivElement>(null),
    [columns, setColumns] = useState(5);
  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    const resize = () => {
      const w = el.clientWidth;
      setColumns(w >= 960 ? 5 : w >= 650 ? 4 : w >= 330 ? 3 : 2);
    };
    resize();
    const observer = new ResizeObserver(resize);
    observer.observe(el);
    return () => observer.disconnect();
  }, []);
  return (
    <div
      ref={ref}
      className="masonry"
      style={{
        display: "grid",
        gridTemplateColumns: `repeat(${columns},minmax(0,1fr))`,
        gap: 12,
        alignItems: "start",
      }}
    >
      {Array.from({ length: columns }, (_, column) => (
        <div key={column}>
          {items.map((w, i) =>
            i % columns === column ? (
              <WallpaperCard key={w.id} w={w} index={i} />
            ) : null,
          )}
        </div>
      ))}
    </div>
  );
}

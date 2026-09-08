export type ImageFile = {
  id: string;
  url: string;
  thumbnail: string;
  width: number;
  height: number;
  bytes: number;
  mime: string;
};
export type Entity = {
  id: string;
  kind: "author" | "topic" | "channel";
  name: string;
  description?: string;
  avatar?: string;
  cover?: string;
  handle?: string;
  featured?: boolean;
  subscribed?: boolean;
  subscribers?: number;
  count?: number;
  score?: number;
};
export type Wallpaper = {
  id: string;
  title: string;
  description: string;
  tags: string[];
  topicIds: string[];
  sourceUrl: string;
  sourceMetrics: Record<string, number>;
  publishedAt: string;
  status: string;
  featured: boolean;
  author: Entity;
  channel: Entity;
  images: ImageFile[];
  likes: number;
  favorites: number;
  downloads: number;
  liked: boolean;
  favorited: boolean;
  score?: number;
};
export type Page<T> = { items: T[]; nextCursor?: string };
export type Profile = {
  name: string;
  bio: string;
  avatar: string;
  cover: string;
};
export type User = {
  id: string;
  username: string;
  role: string;
  profile: Profile;
  stats: {
    favorites: number;
    downloads: number;
    subscriptions: number;
    history: number;
  };
};
export type Session = { user: User | null; csrf: string; demo: boolean };
let csrf = "";
export function setCSRF(token: string) {
  csrf = token;
}
export class APIError extends Error {
  status: number;
  constructor(message: string, status: number) {
    super(message);
    this.status = status;
  }
}
export async function api<T>(
  path: string,
  method = "GET",
  body?: unknown,
): Promise<T> {
  const response = await fetch("/api/v1" + path, {
    method,
    credentials: "same-origin",
    headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (!response.ok) {
    const data = await response
      .json()
      .catch(() => ({ error: "网络请求失败，请稍后重试" }));
    throw new APIError(data.error || "请求失败", response.status);
  }
  return response.json();
}
export async function downloadFile(w: Wallpaper, img: ImageFile) {
  const r = await fetch(`/api/v1/wallpapers/${w.id}/download/${img.id}`, {
    method: "POST",
    headers: { "X-CSRF-Token": csrf },
  });
  if (!r.ok) {
    const d = await r.json();
    throw new APIError(d.error, r.status);
  }
  const blob = await r.blob();
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = `${w.title}.${img.mime.split("/")[1] || "jpg"}`;
  a.click();
  setTimeout(() => URL.revokeObjectURL(url), 10000);
}
export function params(values: Record<string, string | undefined>) {
  return new URLSearchParams(
    Object.entries(values).filter((v): v is [string, string] => !!v[1]),
  ).toString();
}
export function number(n = 0) {
  return n >= 10000 ? (n / 10000).toFixed(1) + "w" : n.toLocaleString("zh-CN");
}

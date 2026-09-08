import React from "react";
import ReactDOM from "react-dom/client";
import { BrowserRouter, Routes, Route, useLocation } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MotionConfig } from "motion/react";
import { AppProvider } from "./context";
import { Header, Footer } from "./components";
import {
  HomePage,
  DiscoverPage,
  DetailPage,
  RankingPage,
  EntitiesPage,
  EntityPage,
} from "./pages";
import { MyPage } from "./my";
import { AdminPage } from "./admin";
import "./style.css";
import "./extra.css";
const client = new QueryClient({
  defaultOptions: {
    queries: { staleTime: 30000, retry: 1, refetchOnWindowFocus: false },
  },
});
function ScrollReset() {
  const { pathname } = useLocation();
  React.useEffect(() => {
    // Some browsers return a Promise from scrollTo; effects may only return cleanup functions.
    window.scrollTo(0, 0);
  }, [pathname]);
  return null;
}
function App() {
  return (
    <QueryClientProvider client={client}>
      <BrowserRouter>
        <MotionConfig reducedMotion="user">
          <AppProvider>
            <ScrollReset />
            <Header />
            <main className="page">
              <Routes>
                <Route path="/" element={<HomePage />} />
                <Route path="/discover" element={<DiscoverPage />} />
                <Route path="/wallpaper/:id" element={<DetailPage />} />
                <Route path="/ranking" element={<RankingPage />} />
                <Route
                  path="/authors"
                  element={<EntitiesPage kind="author" />}
                />
                <Route path="/authors/:id" element={<EntityPage />} />
                <Route path="/topics" element={<EntitiesPage kind="topic" />} />
                <Route path="/topics/:id" element={<EntityPage />} />
                <Route path="/channels/:id" element={<EntityPage />} />
                <Route path="/me/:tab?" element={<MyPage />} />
                <Route path="/admin/:tab?" element={<AdminPage />} />
                <Route
                  path="*"
                  element={
                    <div className="empty">
                      <h1>这一页漂流到了宇宙之外</h1>
                      <a href="/">返回首页</a>
                    </div>
                  }
                />
              </Routes>
            </main>
            <Footer />
          </AppProvider>
        </MotionConfig>
      </BrowserRouter>
    </QueryClientProvider>
  );
}
ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
);

import { test, expect } from "@playwright/test";

test("client navigation remains visible when scrollTo returns a Promise", async ({
  page,
}) => {
  await page.addInitScript(() => {
    window.scrollTo = (() =>
      Promise.resolve()) as unknown as typeof window.scrollTo;
  });
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  await page.goto("/");
  // No goto/reload between routes: the original bug only appeared on client navigation.
  for (const label of [
    "发现",
    "排行",
    "作者",
    "专题",
    "首页",
    "发现",
    "首页",
  ]) {
    await page
      .locator(".desktop-nav")
      .getByRole("link", { name: label, exact: true })
      .click();
    await expect(page.locator("main")).toBeVisible();
    await expect(page.locator("main h1, main h2").first()).toBeVisible();
    expect(errors).toEqual([]);
  }
  await page.goBack();
  await expect(page.locator("main")).toBeVisible();
  await page.goForward();
  await expect(page.locator("main")).toBeVisible();
  expect(errors).toEqual([]);
});

test("admin links, image quality presets and paid-run cancellation", async ({
  page,
  request,
  baseURL,
}) => {
  test.skip(!process.env.ADMIN_PASSWORD, "Administrator credentials required");
  const login = await request.post("/api/v1/auth/login", {
    headers: { Origin: baseURL! },
    data: {
      username: process.env.ADMIN_USERNAME || "admin",
      password: process.env.ADMIN_PASSWORD,
    },
  });
  expect(login.ok()).toBeTruthy();
  await page.context().addCookies((await request.storageState()).cookies);
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  let paidCalls = 0;
  await page.route("**/api/v1/admin/sources/*/run", async (route) => {
    paidCalls++;
    await route.fulfill({
      status: 400,
      json: { error: "No paid runs in tests" },
    });
  });
  await page.goto("/admin/sources");
  for (const label of [
    "运行概览",
    "Apify 配置",
    "采集来源",
    "任务记录",
    "作品管理",
    "作者与主题",
    "首页配置",
    "用户管理",
    "采集来源",
  ]) {
    await page
      .locator(".admin-nav")
      .getByRole("link", { name: label, exact: true })
      .click();
    await expect(
      page.locator(".admin-main h1, .admin-main h2").first(),
    ).toBeVisible();
    expect(errors).toEqual([]);
  }
  await page.getByRole("button", { name: "添加来源" }).click();
  await page.getByLabel("作者列表（2–100 个用户名）").fill(Array.from({length:100},(_,i)=>"artist"+i).join(","));
  await page.getByRole("button",{name:"预览批量查询（免费）"}).click();
  await expect(page.locator("pre")).toContainText('"queryType": "Latest"');
  const preview=JSON.parse(await page.locator("pre").innerText());
  expect(preview.searchTerms).toHaveLength(5);
  expect(preview.maxItems).toBe(100);
  expect(preview.twitterHandles).toBeUndefined();
  await expect(page.getByLabel("图片清晰度")).toHaveValue("standard");
  await expect(page.getByLabel("最小短边（像素）")).toHaveCount(0);
  await expect(page.getByLabel("启用定时采集并自动发布")).not.toBeChecked();
  await page.getByLabel("图片清晰度").selectOption("high");
  await page.getByLabel("图片清晰度").selectOption("custom");
  await expect(page.getByLabel("最小短边（像素）")).toHaveValue("1080");
  await expect(page.getByLabel("最小长边（像素）")).toHaveValue("1920");
  await page.getByLabel("图片清晰度").selectOption("all");
  await expect(page.getByLabel("最小短边（像素）")).toHaveCount(0);
  await page.getByRole("dialog").getByRole("button", { name: "关闭" }).click();
  await page
    .locator("button:not([disabled])").filter({hasText: "采集（付费）"})
    .first()
    .click();
  await expect(
    page.getByRole("dialog", { name: "确认付费采集" }),
  ).toBeVisible();
  await expect(page.getByRole("dialog")).toContainText("另计平台用量");
  await page.getByRole("button", { name: "取消", exact: true }).click();
  expect(paidCalls).toBe(0);
  expect(errors).toEqual([]);
});

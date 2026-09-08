import { test, expect } from "@playwright/test";
import fs from "node:fs";
test("responsive prototype pages and image integrity", async ({ page }) => {
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  for (const width of [1440, 1024, 390]) {
    await page.setViewportSize({ width, height: 1080 });
    for (const [path, name] of [
      ["/", "home"],
      ["/wallpaper/demo-lake", "detail"],
      ["/ranking", "ranking"],
    ]) {
      await page.goto(path);
      await expect(page.locator(".site-header")).toBeVisible();
      await page.waitForLoadState("networkidle");
      await expect(page.locator(".empty h3")).toHaveCount(0);
      expect(
        await page.evaluate(
          () => document.documentElement.scrollWidth <= window.innerWidth,
        ),
      ).toBeTruthy();
      expect(
        await page
          .locator("img")
          .evaluateAll((imgs) =>
            imgs.every(
              (i) =>
                (i as HTMLImageElement).complete &&
                (i as HTMLImageElement).naturalWidth > 0,
            ),
          ),
      ).toBeTruthy();
      await page.screenshot({
        path: `../artifacts/${name}-${width}.png`,
        fullPage: true,
      });
    }
  }
  expect(errors).toEqual([]);
});
test("registration, favorites, likes, subscription, search, download and profile", async ({
  page,
}) => {
  const username = "test_" + Date.now(),
    password = "Test-password-" + Date.now();
  await page.goto("/");
  await page.getByRole("button", { name: "登录", exact: true }).click();
  await page.getByRole("button", { name: "立即注册", exact: true }).click();
  await page.getByLabel("账号", { exact: true }).fill(username);
  await page.getByLabel("密码", { exact: true }).fill(password);
  await page.getByRole("button", { name: "创建账号", exact: true }).click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await page.goto("/wallpaper/demo-lake");
  await expect(page.getByRole("heading", { name: /日照金山/ })).toBeVisible();
  await page.getByRole("button", { name: "查看第 2 张" }).click();
  await expect(page.locator(".image-counter")).toHaveText("2/4");
  await page.getByRole("button", { name: "全屏预览" }).click();
  await expect(
    page.getByRole("dialog", { name: "全屏图片预览" }),
  ).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await page.getByRole("button", { name: "收藏", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "已收藏", exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "喜欢壁纸" }).click();
  await expect(page.getByRole("button", { name: "喜欢壁纸" })).toHaveClass(
    "liked",
  );
  await page.getByRole("button", { name: "关注", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "已订阅", exact: true }),
  ).toBeVisible();
  const downloadPromise = page.waitForEvent("download");
  await page.getByRole("button", { name: "下载", exact: true }).click();
  const download = await downloadPromise;
  expect(download.suggestedFilename()).toContain("日照金山");
  const file = await download.path();
  expect(fs.statSync(file!).size).toBeGreaterThan(10000);
  await page.goto("/me/favorites");
  await expect(
    page.getByRole("link", { name: "日照金山", exact: true }),
  ).toBeVisible();
  await page.goto("/me/downloads");
  await expect(
    page.getByRole("link", { name: "日照金山", exact: true }),
  ).toBeVisible();
  await page.goto("/discover?scope=subscriptions");
  await expect(
    page.getByRole("link", { name: "日照金山", exact: true }),
  ).toBeVisible();
  await page.goto("/discover?q=不存在的壁纸标题");
  await expect(page.getByText("没有找到相关壁纸")).toBeVisible();
  await page.goto("/me");
  await page.getByRole("button", { name: "编辑资料", exact: true }).click();
  await page.getByLabel("昵称", { exact: true }).fill("林间向风");
  await page
    .getByLabel("个人简介", { exact: true })
    .fill("让平凡的日子也闪闪发光");
  await page.getByRole("button", { name: "保存修改" }).click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await expect(page.getByRole("heading", { name: "林间向风" })).toBeVisible();
  for (const width of [1440, 1024, 390]) {
    await page.setViewportSize({ width, height: 1080 });
    await page.waitForLoadState("networkidle");
    await page.screenshot({
      path: `../artifacts/profile-${width}.png`,
      fullPage: true,
    });
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= window.innerWidth,
      ),
    ).toBeTruthy();
  }
  await page.goto("/admin");
  await expect(page.getByText("需要管理员权限")).toBeVisible();
});
test("admin source management and no secret exposure", async ({ page }) => {
  test.skip(
    !process.env.ADMIN_PASSWORD,
    "Set ADMIN_PASSWORD to run administrator checks",
  );
  await page.goto("/admin");
  await page.getByRole("button", { name: "登录", exact: true }).last().click();
  await page
    .getByLabel("账号", { exact: true })
    .fill(process.env.ADMIN_USERNAME || "admin");
  await page
    .getByLabel("密码", { exact: true })
    .fill(process.env.ADMIN_PASSWORD!);
  await page.getByRole("button", { name: "登录", exact: true }).last().click();
  await expect(page.getByRole("heading", { name: "运行概览" })).toBeVisible();
  await page.goto("/admin/settings");
  await expect(page.getByText("未配置", { exact: true })).toBeVisible();
  expect(await page.locator("body").innerText()).not.toContain(
    process.env.ADMIN_PASSWORD!,
  );
  await page.goto("/admin/sources");
  await page.getByRole("button", { name: "添加来源" }).click();
  await page.getByLabel("来源名称").fill("验证来源 " + Date.now());
  await page.getByLabel("作者列表（2–100 个用户名）").fill("NASA, NASAWebb");
  await page.getByRole("button", { name: "保存来源" }).click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await expect(page.getByText("NASA NASAWebb", { exact: true }).last()).toBeVisible();
  await page.screenshot({
    path: "../artifacts/admin-sources.png",
    fullPage: true,
  });
  await page.goto("/admin/content");
  await expect(page.getByRole("table")).toBeVisible();
  await page.getByRole("button", { name: "编辑", exact: true }).first().click();
  await expect(page.getByRole("dialog", { name: "编辑作品" })).toBeVisible();
  await page.getByRole("button", { name: "保存修改" }).click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
});

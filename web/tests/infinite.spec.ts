import { test, expect } from "@playwright/test";

test("wallpaper grids load the next page on scroll without a button", async ({page,request})=>{
 const sample=(await (await request.get('/api/v1/wallpapers?limit=1')).json()).items[0];
 expect(sample).toBeTruthy();
 const calls:string[]=[];
 await page.route('**/api/v1/wallpapers?**',async route=>{
  const url=new URL(route.request().url());
  if(url.searchParams.get('topic')!=='scroll-test')return route.continue();
  const cursor=url.searchParams.get('cursor')||'';calls.push(cursor);
  expect(url.searchParams.has('featured')).toBe(false);
  await route.fulfill({json:{items:Array.from({length:24},(_,i)=>({...sample,id:`scroll-${cursor||'first'}-${i}`})),nextCursor:cursor?'':'second'}});
 });
 await page.goto('/?topic=scroll-test');
 await expect(page.locator('.masonry article')).toHaveCount(24);
 await page.locator('.load-more').scrollIntoViewIfNeeded();
 await expect(page.locator('.masonry article')).toHaveCount(48);
 await expect(page.getByText('已展示全部壁纸')).toBeVisible();
 expect(calls).toEqual(['','second']);
 await expect(page.getByRole('button',{name:'加载更多壁纸',exact:true})).toHaveCount(0);
});

test("failed next page keeps existing wallpapers and supports retry",async({page,request})=>{
 const sample=(await(await request.get('/api/v1/wallpapers?limit=1')).json()).items[0];
 let fail=true;
 await page.route('**/api/v1/wallpapers?**',async route=>{
  const url=new URL(route.request().url());
  if(url.searchParams.get('topic')!=='retry-test')return route.continue();
  const cursor=url.searchParams.get('cursor');
  if(cursor && fail)return route.fulfill({status:500,json:{error:'temporary failure'}});
  await route.fulfill({json:{items:Array.from({length:24},(_,i)=>({...sample,id:`retry-${cursor||'first'}-${i}`})),nextCursor:cursor?'':'second'}});
 });
 await page.goto('/?topic=retry-test');
 await expect(page.locator('.masonry article')).toHaveCount(24);
 await page.locator('.load-more').scrollIntoViewIfNeeded();
 await expect(page.getByRole('button',{name:'加载失败，点击重试'})).toBeVisible();
 await expect(page.locator('.masonry article')).toHaveCount(24);
 fail=false;
 await page.getByRole('button',{name:'加载失败，点击重试'}).click();
 await expect(page.locator('.masonry article')).toHaveCount(48);
});

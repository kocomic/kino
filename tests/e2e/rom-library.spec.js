const { test, expect } = require('@playwright/test');
test('ROM-only UI: create, edit, add version, upload media, and remove records', async ({page}) => {
 const failed = []; page.on('pageerror', e => failed.push(e.message));
 await page.goto('/'); await expect(page.locator('#connection')).toHaveText('服务已连接');
 await expect(page.getByRole('navigation')).not.toContainText(/联机|存档|模拟器|设备/);
 await page.getByRole('button',{name:'添加游戏',exact:true}).click();
 await page.getByLabel('游戏名称').fill('Browser ROM fixture'); await page.getByRole('button',{name:'创建',exact:true}).click();
 await expect(page.getByRole('dialog')).toContainText('Browser ROM fixture');
 await page.getByLabel('游戏名称').fill('Updated fixture'); await page.getByRole('button',{name:'保存修改'}).click();
 page.once('dialog',d=>d.accept('Fixture edition')); await page.getByRole('button',{name:'添加版本'}).click(); await expect(page.getByRole('dialog')).toContainText('Fixture edition');
 await page.getByLabel('上传封面').setInputFiles({name:'cover.png',mimeType:'image/png',buffer:Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aX2kAAAAASUVORK5CYII=','base64')});
 await page.getByRole('button',{name:'上传',exact:true}).click(); await expect(page.getByRole('dialog')).not.toBeVisible(); await expect(page.locator('.game')).toContainText('Updated fixture');
 await page.locator('.game').filter({hasText:'Updated fixture'}).click(); page.once('dialog',d=>d.accept()); await page.getByRole('button',{name:'删除游戏',exact:true}).click(); await expect(page.locator('.game').filter({hasText:'Updated fixture'})).toHaveCount(0);
 expect(failed).toEqual([]);
});
test('scan and explicitly import ROMs; no removed routes fetched', async ({page}) => {
 const requests=[];page.on('request',r=>requests.push(r.url()));await page.goto('/');await expect(page.locator('#connection')).toHaveText('服务已连接');
 await page.getByRole('button',{name:'导入 ROM',exact:true}).click();await page.getByLabel('来源路径').fill('pegasus/gba');await page.locator('#import-platform').selectOption('gba');
 await page.getByRole('button',{name:'扫描并预览'}).click();await expect(page.locator('#preview')).toBeVisible();await expect(page.locator('#candidates input:checked').first()).toBeVisible();await page.getByRole('button',{name:'导入所选项目'}).click();await expect(page.locator('#library')).toBeVisible();await expect(page.locator('.game').first()).toBeVisible();
 expect(requests.filter(u=>/\/api\/.*(saves|sync|devices|web-emulation|web-netplay|packages)/.test(u))).toEqual([]);
 await page.getByRole('button',{name:'深色',exact:true}).click();await expect(page.locator('html')).toHaveAttribute('data-theme','dark');
 await page.setViewportSize({width:390,height:844});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);
});

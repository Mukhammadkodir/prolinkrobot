const { chromium } = require('playwright');
const fs = require('fs');

(async () => {
  const raw = JSON.parse(fs.readFileSync('/Users/mukhammadkodir/Documents/Codes/prolinkrobot/freepik_cookies.json', 'utf8'));
  const cookiesObj = raw['Request Cookies'] || raw.cookies || raw;
  const cookies = Object.entries(cookiesObj).map(([name, value]) => ({
    name,
    value: String(value),
    domain: '.freepik.com',
    path: '/',
    secure: true,
    httpOnly: false,
    sameSite: 'Lax',
  }));

  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({ viewport: { width: 1440, height: 900 } });
  await context.addCookies(cookies);
  const page = await context.newPage();

  page.on('request', req => {
    const url = req.url();
    if (/3d|download|api|model/i.test(url)) {
      console.log('REQ', req.method(), url);
      const pd = req.postData();
      if (pd) console.log('POST', pd.slice(0, 500));
    }
  });
  page.on('response', async res => {
    const url = res.url();
    if (/3d|download|api|model/i.test(url)) {
      console.log('RES', res.status(), url);
      const ct = res.headers()['content-type'] || '';
      if (ct.includes('application/json') || ct.includes('text/plain')) {
        try {
          const txt = await res.text();
          console.log('BODY', txt.slice(0, 1500));
        } catch {}
      }
    }
  });

  await page.goto('https://www.freepik.com/3d-model/tube-box-with-label_23192.htm#fromView=keyword&page=1&position=0&uuid=4085ca76-a1f2-4d7e-abca-34c8a791ac91&track=3d', { waitUntil: 'networkidle', timeout: 90000 });
  console.log('TITLE', await page.title());
  console.log('URL', page.url());
  console.log('BODYTEXT', (await page.locator('body').innerText()).slice(0, 3000));

  const buttons = await page.locator('button, a').evaluateAll(nodes => nodes.map(n => ({ text: (n.innerText || n.textContent || '').trim(), href: n.href || '', cls: String(n.className || '') })).filter(x => /blend|obj|fbx|textures|download/i.test(x.text + ' ' + x.href + ' ' + x.cls)).slice(0, 80));
  console.log('BUTTONS', JSON.stringify(buttons, null, 2));

  for (const txt of ['Download', 'BLEND', 'OBJ', 'FBX', 'TEXTURES']) {
    const loc = page.getByText(txt, { exact: false }).first();
    if (await loc.count()) {
      console.log('CLICK', txt);
      try { await loc.click({ timeout: 5000 }); } catch (e) { console.log('CLICKERR', String(e)); }
      await page.waitForTimeout(2000);
    }
  }

  await browser.close();
})();

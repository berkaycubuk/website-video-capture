#!/usr/bin/env node
/*
 * High-resolution region screenshots.
 * Captures specific parts of pages (by CSS selector or explicit clip) at a high
 * deviceScaleFactor for crisp, zoomable stills. Live pages can be advanced first
 * (clock control) so canvas animations fill in before the shot.
 *
 * Usage:
 *   node shots.js                       # all shots, all themes
 *   node shots.js --theme light
 *   node shots.js --only home-headline,home-gameoflife
 *   node shots.js --scale 4             # override deviceScaleFactor
 *   node shots.js --base http://localhost:3000
 */
const { chromium } = require('playwright');
const fs = require('fs');
const path = require('path');
const cfg = require('./config.js');

const argv = process.argv.slice(2);
const opt = (n, d) => { const i = argv.indexOf(n); return i >= 0 ? argv[i + 1] : d; };
const flag = n => argv.includes(n);

const BASE = opt('--base', cfg.baseUrl);
const THEMES = opt('--theme', 'all') === 'all' ? cfg.themes : opt('--theme', 'all').split(',');
const ONLY = opt('--only', null) ? opt('--only', null).split(',') : null;
const SCALE_OVERRIDE = opt('--scale', null) ? Number(opt('--scale', null)) : null;

const SD = cfg.shotDefaults || { viewport: { width: 1600, height: 1000 }, deviceScaleFactor: 3, pad: 0 };
const OUT = path.resolve(__dirname, cfg.outDir, 'shots');
fs.mkdirSync(OUT, { recursive: true });

const sleep = ms => new Promise(r => setTimeout(r, ms));
const suffix = theme => (theme === cfg.defaultTheme ? '' : `_${theme}`);
const shotsToRun = () => (ONLY ? cfg.shots.filter(s => ONLY.includes(s.name)) : cfg.shots);

async function captureTheme(theme) {
  const suf = suffix(theme);
  const isDark = theme === cfg.theme.darkValue;
  // group by scale so we relaunch contexts only when needed
  for (const shot of shotsToRun()) {
    const dsf = SCALE_OVERRIDE || shot.scale || SD.deviceScaleFactor;
    const browser = await chromium.launch({ headless: true });
    const context = await browser.newContext({
      viewport: SD.viewport,
      deviceScaleFactor: dsf,
      colorScheme: isDark ? 'dark' : 'light',
      reducedMotion: 'no-preference',
    });
    await context.addInitScript(([k, v]) => { try { localStorage.setItem(k, v); } catch (e) {} },
      [cfg.theme.storageKey, theme]);

    const page = await context.newPage();
    if (shot.live) await page.clock.install({ time: 0 });
    await page.goto(BASE + shot.path, { waitUntil: 'networkidle' });
    await page.evaluate(([cls, dark]) => {
      document.documentElement.classList.toggle(cls, dark);
      window.scrollTo(0, 0);
    }, [cfg.theme.darkClass, isDark]);
    await page.evaluate(() => document.fonts && document.fonts.ready).catch(() => {});

    if (shot.live && shot.advanceMs) {
      await page.clock.runFor(shot.advanceMs);   // let the animation fill in
      await sleep(80);                            // let the canvas composite
    }

    const out = path.join(OUT, `${shot.name}${suf}.png`);
    const pad = shot.pad != null ? shot.pad : SD.pad;

    if (shot.clip) {
      const c = shot.clip;
      await page.screenshot({ path: out, clip: c });
    } else if (shot.selector) {
      let loc = page.locator(shot.selector);
      if (shot.nth != null) loc = loc.nth(shot.nth);
      if (pad > 0) {
        const box = await loc.boundingBox();
        await page.screenshot({ path: out, clip: {
          x: Math.max(0, box.x - pad), y: Math.max(0, box.y - pad),
          width: box.width + pad * 2, height: box.height + pad * 2,
        } });
      } else {
        await loc.screenshot({ path: out });
      }
    } else {
      await page.screenshot({ path: out }); // full viewport
    }

    const dim = await page.evaluate(() => 0); // noop
    console.log(`  ${shot.name}${suf}.png  @${dsf}x`);
    await browser.close();
  }
}

(async () => {
  console.log(`shots: ${BASE}  themes=[${THEMES}]  scale=${SCALE_OVERRIDE || SD.deviceScaleFactor}x`);
  for (const theme of THEMES) {
    console.log(`capturing shots (${theme})...`);
    await captureTheme(theme);
  }
  console.log(`done -> ${OUT}`);
})().catch(e => { console.error(e); process.exit(1); });

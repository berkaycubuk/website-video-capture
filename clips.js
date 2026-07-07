#!/usr/bin/env node
/*
 * Region video clips.
 * Records a fixed rectangular region of a page as an mp4. Ideal for animated
 * areas (e.g. Conway's Game of Life): with `live: true` the page clock advances
 * exactly 1/fps per frame, so the animation plays at true speed, deterministically.
 *
 * Usage:
 *   node clips.js                     # all clips, all themes
 *   node clips.js --theme light
 *   node clips.js --only gameoflife
 *   node clips.js --seconds 12        # override duration
 *   node clips.js --base http://localhost:3000
 */
const { chromium } = require('playwright');
const fs = require('fs');
const path = require('path');
const { spawnSync } = require('child_process');
const cfg = require('./config.js');

const argv = process.argv.slice(2);
const opt = (n, d) => { const i = argv.indexOf(n); return i >= 0 ? argv[i + 1] : d; };

const BASE = opt('--base', cfg.baseUrl);
const THEMES = opt('--theme', 'all') === 'all' ? cfg.themes : opt('--theme', 'all').split(',');
const ONLY = opt('--only', null) ? opt('--only', null).split(',') : null;
const SECONDS_OVERRIDE = opt('--seconds', null) ? Number(opt('--seconds', null)) : null;

const CD = cfg.clipDefaults;
const FPS = CD.fps;
const DT = 1000 / FPS;

const OUTV = path.resolve(__dirname, cfg.outDir, 'clips');
const OUTF = path.resolve(__dirname, cfg.outDir, 'clip-frames');
fs.mkdirSync(OUTV, { recursive: true });

const sleep = ms => new Promise(r => setTimeout(r, ms));
const suffix = theme => (theme === cfg.defaultTheme ? '' : `_${theme}`);
const clipsToRun = () => (ONLY ? cfg.clips.filter(c => ONLY.includes(c.name)) : cfg.clips);

function ffmpeg(args) {
  const r = spawnSync('ffmpeg', args, { encoding: 'utf8' });
  if (r.status !== 0) throw new Error('ffmpeg failed: ' + (r.stderr || r.stdout));
}

async function captureTheme(theme) {
  const suf = suffix(theme);
  const isDark = theme === cfg.theme.darkValue;
  for (const clip of clipsToRun()) {
    const dsf = clip.scale || CD.deviceScaleFactor;
    const seconds = SECONDS_OVERRIDE || clip.seconds || CD.seconds;
    const browser = await chromium.launch({ headless: true });
    const context = await browser.newContext({
      viewport: CD.viewport,
      deviceScaleFactor: dsf,
      colorScheme: isDark ? 'dark' : 'light',
      reducedMotion: 'no-preference',
    });
    await context.addInitScript(([k, v]) => { try { localStorage.setItem(k, v); } catch (e) {} },
      [cfg.theme.storageKey, theme]);

    const page = await context.newPage();
    if (clip.live) await page.clock.install({ time: 0 });
    await page.goto(BASE + clip.path, { waitUntil: 'networkidle' });
    await page.evaluate(([cls, dark]) => {
      document.documentElement.classList.toggle(cls, dark);
      window.scrollTo(0, 0);
    }, [cfg.theme.darkClass, isDark]);
    await page.evaluate(() => document.fonts && document.fonts.ready).catch(() => {});

    // Optional: click element(s) before recording (e.g. a "Random" reseed button).
    if (clip.clickBefore) {
      for (const sel of [].concat(clip.clickBefore)) {
        await page.click(sel).catch(e => { throw new Error(`clickBefore "${sel}": ${e.message}`); });
      }
    }

    // Fixed capture box (CSS px). Same box every frame -> stable dimensions.
    let box = clip.clip;
    if (!box) {
      let loc = page.locator(clip.selector);
      if (clip.nth != null) loc = loc.nth(clip.nth);
      const b = await loc.boundingBox();
      if (!b) throw new Error(`clip "${clip.name}": selector not found / not visible: ${clip.selector}`);
      box = { x: Math.round(b.x), y: Math.round(b.y), width: Math.round(b.width), height: Math.round(b.height) };
    }

    const fdir = path.join(OUTF, `${clip.name}${suf}`);
    fs.rmSync(fdir, { recursive: true, force: true });
    fs.mkdirSync(fdir, { recursive: true });

    const total = Math.round(seconds * FPS);
    for (let i = 0; i < total; i++) {
      if (clip.live) { await page.clock.runFor(DT); await sleep(CD.compositeSleepMs); }
      await page.screenshot({ path: path.join(fdir, `f_${String(i + 1).padStart(5, '0')}.png`), clip: box });
    }
    await browser.close();

    const out = path.join(OUTV, `${clip.name}${suf}.mp4`);
    ffmpeg(['-y', '-v', 'error', '-framerate', String(FPS), '-i', path.join(fdir, 'f_%05d.png'),
      // crop to even dimensions (H.264 yuv420p requirement)
      '-vf', 'crop=trunc(iw/2)*2:trunc(ih/2)*2,format=yuv420p',
      '-c:v', 'libx264', '-crf', String(cfg.encode.crf), '-preset', cfg.encode.preset,
      '-pix_fmt', 'yuv420p', '-movflags', '+faststart', out]);
    console.log(`  ${clip.name}${suf}.mp4  ${box.width * dsf}x${box.height * dsf}, ${seconds}s @${FPS}fps`);
  }
}

(async () => {
  console.log(`clips: ${BASE}  themes=[${THEMES}]  @${FPS}fps`);
  for (const theme of THEMES) { console.log(`recording clips (${theme})...`); await captureTheme(theme); }
  console.log(`done -> ${OUTV}`);
})().catch(e => { console.error(e); process.exit(1); });

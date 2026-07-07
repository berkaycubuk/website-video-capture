#!/usr/bin/env node
/*
 * website-video-capture
 * Sharp 4K/60fps scrolling videos of website pages.
 *  - True retina via deviceScaleFactor.
 *  - Live canvas animations captured deterministically via Playwright clock control.
 *  - Static pages: retina full-page screenshot + ffmpeg eased crop-pan.
 *
 * Usage:
 *   node index.js                     # all themes, all pages, capture + assemble
 *   node index.js --theme light       # one theme
 *   node index.js --only home,about   # subset of pages
 *   node index.js --base http://localhost:3000
 *   node index.js --no-assemble       # capture frames/stills only
 *   node index.js --assemble-only     # (re)assemble from existing captures
 */
const { chromium } = require('playwright');
const fs = require('fs');
const path = require('path');
const { spawnSync } = require('child_process');
const cfg = require('./config.js');

// ---- args ----
const argv = process.argv.slice(2);
const opt = (name, def) => { const i = argv.indexOf(name); return i >= 0 ? argv[i + 1] : def; };
const flag = name => argv.includes(name);
if (flag('--help') || flag('-h')) { console.log(fs.readFileSync(__filename, 'utf8').split('*/')[0].replace('#!/usr/bin/env node', '').trim()); process.exit(0); }

const BASE = opt('--base', cfg.baseUrl);
const THEMES = opt('--theme', 'all') === 'all' ? cfg.themes : opt('--theme', 'all').split(',');
const ONLY = opt('--only', null) ? opt('--only', null).split(',') : null;
const DO_CAPTURE = !flag('--assemble-only');
const DO_ASSEMBLE = !flag('--no-assemble');
const FPS = cfg.fps;
const DT = 1000 / FPS;
const [VW, VH] = [cfg.viewport.width, cfg.viewport.height];
const OW = VW * cfg.deviceScaleFactor, OH = VH * cfg.deviceScaleFactor; // output pixel size

const OUT = path.resolve(__dirname, cfg.outDir);
const D_FRAMES = path.join(OUT, 'frames');
const D_STILLS = path.join(OUT, 'stills');
const D_VIDEOS = path.join(OUT, 'videos');
[OUT, D_FRAMES, D_STILLS, D_VIDEOS].forEach(d => fs.mkdirSync(d, { recursive: true }));

const ease = t => (t < 0.5 ? 2 * t * t : 1 - Math.pow(-2 * t + 2, 2) / 2);
const sleep = ms => new Promise(r => setTimeout(r, ms));
const suffix = theme => (theme === cfg.defaultTheme ? '' : `_${theme}`);
const pagesToRun = () => (ONLY ? cfg.pages.filter(p => ONLY.includes(p.name)) : cfg.pages);

function run(cmd, args) {
  const r = spawnSync(cmd, args, { encoding: 'utf8' });
  if (r.status !== 0) throw new Error(`${cmd} failed: ${r.stderr || r.stdout}`);
  return r.stdout;
}
function imgSize(file) {
  const out = run('ffprobe', ['-v', 'error', '-select_streams', 'v:0',
    '-show_entries', 'stream=width,height', '-of', 'csv=p=0', file]).trim();
  const [w, h] = out.split(',').map(Number);
  return { w, h };
}

// ---- capture ----
async function captureTheme(theme) {
  const suf = suffix(theme);
  const isDark = theme === cfg.theme.darkValue;
  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({
    viewport: { width: VW, height: VH },
    deviceScaleFactor: cfg.deviceScaleFactor,
    colorScheme: isDark ? 'dark' : 'light',
    reducedMotion: 'no-preference',
  });
  await context.addInitScript(([k, v]) => { try { localStorage.setItem(k, v); } catch (e) {} },
    [cfg.theme.storageKey, theme]);

  for (const pg of pagesToRun()) {
    const page = await context.newPage();
    if (pg.live) await page.clock.install({ time: 0 });
    await page.goto(BASE + pg.path, { waitUntil: 'networkidle' });
    await page.evaluate(([cls, dark]) => {
      document.documentElement.classList.toggle(cls, dark);
      window.scrollTo(0, 0);
    }, [cfg.theme.darkClass, isDark]);
    await page.evaluate(() => document.fonts && document.fonts.ready).catch(() => {});

    const maxY = await page.evaluate(() =>
      Math.max(0, document.documentElement.scrollHeight - window.innerHeight));

    if (pg.live) {
      const fdir = path.join(D_FRAMES, `${pg.name}${suf}`);
      fs.rmSync(fdir, { recursive: true, force: true });
      fs.mkdirSync(fdir, { recursive: true });
      const L = cfg.live;
      const scrollSecs = Math.max(L.minScrollSecs, Math.min(L.maxScrollSecs, maxY / L.scrollPxPerSec));
      const scrollFrames = Math.round(scrollSecs * FPS);
      const total = L.preHoldFrames + scrollFrames + L.postHoldFrames;
      for (let i = 0; i < total; i++) {
        let y;
        if (i < L.preHoldFrames) y = 0;
        else if (i < L.preHoldFrames + scrollFrames) y = maxY * ease((i - L.preHoldFrames) / Math.max(1, scrollFrames - 1));
        else y = maxY;
        await page.evaluate(v => window.scrollTo(0, v), Math.round(y));
        await page.clock.runFor(DT);
        await sleep(L.compositeSleepMs);
        await page.screenshot({ path: path.join(fdir, `f_${String(i + 1).padStart(5, '0')}.png`) });
      }
      console.log(`  live  ${pg.name}${suf}: ${total} frames (maxY=${maxY})`);
    } else {
      const out = path.join(D_STILLS, `${pg.name}${suf}.png`);
      await page.screenshot({ path: out, fullPage: true });
      console.log(`  still ${pg.name}${suf}: captured (maxY=${maxY})`);
    }
    await page.close();
  }
  await browser.close();
}

// ---- assemble ----
function x264(extra) {
  return ['-c:v', 'libx264', '-crf', String(cfg.encode.crf), '-preset', cfg.encode.preset,
    '-pix_fmt', 'yuv420p', '-movflags', '+faststart', ...(extra || [])];
}
function assembleTheme(theme) {
  const suf = suffix(theme);
  for (const pg of pagesToRun()) {
    const out = path.join(D_VIDEOS, `${pg.name}${suf}.mp4`);
    if (pg.live) {
      const fdir = path.join(D_FRAMES, `${pg.name}${suf}`);
      if (!fs.existsSync(fdir)) { console.warn(`  ! missing frames ${fdir}`); continue; }
      run('ffmpeg', ['-y', '-v', 'error', '-framerate', String(FPS),
        '-i', path.join(fdir, 'f_%05d.png'), ...x264(), out]);
      console.log(`  ${pg.name}${suf}.mp4 (live)`);
    } else {
      const img = path.join(D_STILLS, `${pg.name}${suf}.png`);
      if (!fs.existsSync(img)) { console.warn(`  ! missing still ${img}`); continue; }
      const { w, h } = imgSize(img);
      const maxY = h - OH;
      if (maxY <= 0) {
        run('ffmpeg', ['-y', '-v', 'error', '-loop', '1', '-t', String(cfg.pan.staticSecs),
          '-framerate', String(FPS), '-i', img,
          '-vf', `crop=${w}:${OH}:0:0,format=yuv420p`, '-r', String(FPS), ...x264(), out]);
        console.log(`  ${pg.name}${suf}.mp4 (static)`);
      } else {
        const P = cfg.pan;
        const dur = Math.max(P.minSecs, Math.min(P.maxSecs, maxY / P.imgPxPerSec));
        const c = `min(t/${dur}\\,1)`;
        const yexpr = `${maxY}*(3*pow(${c}\\,2)-2*pow(${c}\\,3))`;
        run('ffmpeg', ['-y', '-v', 'error', '-loop', '1', '-t', String(dur),
          '-framerate', String(FPS), '-i', img,
          '-vf', `crop=${w}:${OH}:0:${yexpr},format=yuv420p`, '-r', String(FPS), ...x264(), out]);
        console.log(`  ${pg.name}${suf}.mp4 (pan ${dur.toFixed(1)}s)`);
      }
    }
  }
}

// ---- main ----
(async () => {
  console.log(`webcap: ${BASE}  ->  ${OW}x${OH} @ ${FPS}fps  themes=[${THEMES}]`);
  for (const theme of THEMES) {
    if (DO_CAPTURE) { console.log(`capturing (${theme})...`); await captureTheme(theme); }
    if (DO_ASSEMBLE) { console.log(`assembling (${theme})...`); assembleTheme(theme); }
  }
  console.log(`done -> ${D_VIDEOS}`);
})().catch(e => { console.error(e); process.exit(1); });

// Site + capture configuration. Edit this to target a different site.
module.exports = {
  // Base URL of the running site (dev/preview server or a live URL).
  baseUrl: 'http://localhost:4327',

  // Logical viewport. Final pixel size = viewport * deviceScaleFactor.
  // 1920x1080 @ 2x  ->  3840x2160 (true 4K).
  viewport: { width: 1920, height: 1080 },
  deviceScaleFactor: 2,

  fps: 60,
  outDir: 'out',

  // Light is the default theme (no filename suffix); others get "_<theme>".
  defaultTheme: 'light',
  themes: ['light', 'dark'],

  // How this site expresses theme. The tool sets localStorage[storageKey] and
  // toggles `darkClass` on <html>. `darkValue` is the theme name that is dark.
  theme: { storageKey: 'theme', darkClass: 'dark', darkValue: 'dark' },

  // Encode quality (libx264).
  encode: { crf: 16, preset: 'slow' },

  // Live pages: captured frame-by-frame with the page clock advanced by exactly
  // 1/fps per frame, so canvas animations (e.g. Conway's Game of Life) play at
  // true speed and are perfectly deterministic.
  live: {
    scrollPxPerSec: 300,   // CSS px/sec while scrolling
    minScrollSecs: 4,
    maxScrollSecs: 8,
    preHoldFrames: 42,     // hold at top (~0.7s @60)
    postHoldFrames: 36,    // hold at bottom (~0.6s @60)
    compositeSleepMs: 24,  // real pause so the canvas layer composites before capture
  },

  // Static pages: one retina full-page screenshot, then a smooth 60fps eased
  // crop-pan scroll synthesized with ffmpeg.
  pan: {
    imgPxPerSec: 700,      // image px/sec (image is 2x CSS)
    minSecs: 4,
    maxSecs: 11,
    staticSecs: 5,         // duration when the page fits one screen (no scroll)
  },

  // Pages to capture. `live: true` for pages with running canvas/JS animation.
  pages: [
    { name: 'home',     path: '/',                                     live: true  },
    { name: 'writings', path: '/writings' },
    { name: 'projects', path: '/projects' },
    { name: 'tools',    path: '/tools' },
    { name: 'about',    path: '/about' },
    { name: 'article',  path: '/2026/05/31/live-demo-of-mqtt-studio' },
    { name: 'tool',     path: '/tools/enigma' },
  ],

  // ---- High-res region screenshots (node shots.js) ----
  // Captured at a higher deviceScaleFactor for crisp, zoomable stills.
  shotDefaults: {
    viewport: { width: 1600, height: 1000 },
    deviceScaleFactor: 3,   // 3x = very high resolution
    pad: 0,                 // px of breathing room around the element (CSS px)
  },
  // Each shot: { name, path, selector? , nth?, clip?{x,y,width,height},
  //              live?, advanceMs?, pad?, scale? }
  //  - selector: capture just that element (its bounding box). Omit for full viewport.
  //  - nth: index when the selector matches several elements.
  //  - clip: capture an explicit region instead of a selector.
  //  - live+advanceMs: advance the page clock N ms first (e.g. let Game of Life fill in).
  //  - pad / scale: per-shot overrides.
  shots: [
    { name: 'home-headline',     path: '/', selector: '.masthead' },
    { name: 'home-gameoflife',   path: '/', selector: '.plate',       live: true, advanceMs: 6000 },
    { name: 'home-grid-only',    path: '/', selector: '.plate-inner', live: true, advanceMs: 6000 },
    { name: 'home-topbar',       path: '/', selector: '.topbar' },
    { name: 'home-hero',         path: '/', selector: '.frame',       live: true, advanceMs: 4000 },
    { name: 'footer',            path: '/', selector: '.site-footer' },
    { name: 'writings-masthead', path: '/writings', selector: '.masthead' },
    { name: 'projects-entry',    path: '/projects', selector: '.project', nth: 0 },
  ],

  // ---- Region video clips (node clips.js) ----
  // Records a FIXED region over time as an mp4. Best for animated areas (e.g.
  // the Game of Life): `live: true` advances the page clock 1/fps per frame so
  // the animation plays at true speed. Use a viewport tall enough that the whole
  // region sits above the fold.
  clipDefaults: {
    viewport: { width: 1600, height: 1400 },
    deviceScaleFactor: 2,
    seconds: 8,
    fps: 60,
    compositeSleepMs: 24,
  },
  // Each clip: { name, path, selector? , nth?, clip?{x,y,width,height},
  //             live?, seconds?, scale? }
  clips: [
    { name: 'gameoflife',        path: '/', selector: '.plate',       live: true, seconds: 9 },
    { name: 'gameoflife-grid',   path: '/', selector: '.plate-inner', live: true, seconds: 9 },
    // random-soup seed: click the "Random" button before recording
    { name: 'gameoflife-random', path: '/', selector: '.plate',       live: true, seconds: 10, clickBefore: '#gol-random' },
  ],
};

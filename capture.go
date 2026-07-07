package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"

	"github.com/playwright-community/playwright-go"
)

type captureFlags struct {
	cfg        *Config
	base       string
	themes     []string
	only       []string
	doCapture  bool
	doAssemble bool
}

type dirs struct {
	out, frames, stills, videos string
}

// clockAccumulator carries the fractional-ms remainder of clock advancement
// across frames. playwright-go's Clock.RunFor only accepts integer ticks, so
// we advance by int(ms) per frame and carry the remainder, keeping the
// cumulative advancement exact (matching the JS float DT of 1000/fps).
type clockAccumulator struct{ rem float64 }

func (a *clockAccumulator) RunFor(page playwright.Page, ms float64) error {
	a.rem += ms
	ticks := int(a.rem)
	a.rem -= float64(ticks)
	if ticks <= 0 {
		return nil
	}
	return page.Clock().RunFor(ticks)
}

// triggerLazyLoad scrolls the page viewport-by-viewport so lazy / intersection-
// observer content loads, then returns to the top and waits for images to
// settle. Playwright's fullPage screenshot captures via a clip (no scrolling),
// so without this, below-the-fold images never load and render as blank space.
func triggerLazyLoad(page playwright.Page, maxY, vh int) {
	if maxY <= 0 {
		return
	}
	for y := 0; y <= maxY+vh; y += vh {
		_, _ = page.Evaluate(`v => window.scrollTo(0, v)`, y)
		sleep(150)
	}
	_, _ = page.Evaluate(`() => window.scrollTo(0, 0)`)
	_, _ = page.Evaluate(`() => Promise.race([
		Promise.all([...document.images].map(img => img.complete ? null : new Promise(res => { img.addEventListener('load', res, {once:true}); img.addEventListener('error', res, {once:true}); }))),
		new Promise(res => setTimeout(res, 3000))
	])`)
}

func runCapture(f *captureFlags) error {
	cfg := f.cfg
	FPS := cfg.FPS
	DT := 1000.0 / float64(FPS)
	VW, VH := cfg.Viewport.Width, cfg.Viewport.Height
	OW := int(float64(VW) * cfg.DeviceScaleFactor)
	OH := int(float64(VH) * cfg.DeviceScaleFactor)

	d := dirs{
		out:    abs(cfg.OutDir),
		frames: filepath.Join(abs(cfg.OutDir), "frames"),
		stills: filepath.Join(abs(cfg.OutDir), "stills"),
		videos: filepath.Join(abs(cfg.OutDir), "videos"),
	}
	for _, p := range []string{d.out, d.frames, d.stills, d.videos} {
		if err := os.MkdirAll(p, 0o755); err != nil {
			return err
		}
	}

	pages := filterByName(cfg.Pages, f.only, func(p Page) string { return p.Name })

	fmt.Printf("webcap: %s  ->  %dx%d @ %dfps  themes=[%s]\n", f.base, OW, OH, FPS, joinThemes(f.themes))

	for _, theme := range f.themes {
		if f.doCapture {
			fmt.Printf("capturing (%s)...\n", theme)
			if err := captureTheme(cfg, f.base, theme, pages, VW, VH, FPS, DT, d); err != nil {
				return err
			}
		}
		if f.doAssemble {
			fmt.Printf("assembling (%s)...\n", theme)
			if err := assembleTheme(cfg, theme, pages, FPS, VW, OW, OH, d); err != nil {
				return err
			}
		}
	}
	fmt.Printf("done -> %s\n", d.videos)
	return nil
}

func captureTheme(cfg *Config, base, theme string, pages []Page, VW, VH, FPS int, DT float64, d dirs) error {
	suf := suffix(cfg, theme)
	isDark := theme == cfg.Theme.DarkValue

	pw, err := playwright.Run()
	if err != nil {
		return err
	}
	defer pw.Stop()

	b, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{Headless: playwright.Bool(true)})
	if err != nil {
		return err
	}
	defer b.Close()

	colorScheme := playwright.ColorSchemeLight
	if isDark {
		colorScheme = playwright.ColorSchemeDark
	}
	ctx, err := b.NewContext(playwright.BrowserNewContextOptions{
		Viewport:          &playwright.Size{Width: VW, Height: VH},
		DeviceScaleFactor: playwright.Float(cfg.DeviceScaleFactor),
		ColorScheme:       colorScheme,
		ReducedMotion:     playwright.ReducedMotionReduce,
	})
	if err != nil {
		return err
	}

	if err := addThemeInitScript(ctx, cfg, theme); err != nil {
		return err
	}

	for _, pg := range pages {
		page, err := ctx.NewPage()
		if err != nil {
			return err
		}
		if pg.Live {
			if err := page.Clock().Install(playwright.ClockInstallOptions{Time: 0}); err != nil {
				return err
			}
		}
		if _, err := page.Goto(base+pg.Path, playwright.PageGotoOptions{WaitUntil: playwright.WaitUntilStateNetworkidle}); err != nil {
			return err
		}
		if _, err := page.Evaluate(`([cls, dark]) => { document.documentElement.classList.toggle(cls, dark); window.scrollTo(0, 0); }`,
			[]interface{}{cfg.Theme.DarkClass, isDark}); err != nil {
			return err
		}
		_, _ = page.Evaluate(`() => document.fonts && document.fonts.ready`)
		if pg.ForceVisible {
			applyVisibilityOverride(page)
		}

		res, err := page.Evaluate(`() => Math.max(0, document.documentElement.scrollHeight - window.innerHeight)`)
		if err != nil {
			return err
		}
		maxY := int(toFloat(res))

		if pg.Live {
			fdir := filepath.Join(d.frames, pg.Name+suf)
			_ = os.RemoveAll(fdir)
			if err := os.MkdirAll(fdir, 0o755); err != nil {
				return err
			}
			L := cfg.Live
			scrollSecs := math.Max(float64(L.MinScrollSecs), math.Min(float64(L.MaxScrollSecs), float64(maxY)/float64(L.ScrollPxPerSec)))
			scrollFrames := int(math.Round(scrollSecs * float64(FPS)))
			total := L.PreHoldFrames + scrollFrames + L.PostHoldFrames
			clk := &clockAccumulator{}
			for i := 0; i < total; i++ {
				var y float64
				switch {
				case i < L.PreHoldFrames:
					y = 0
				case i < L.PreHoldFrames+scrollFrames:
					y = float64(maxY) * ease(float64(i-L.PreHoldFrames)/float64(max(1, scrollFrames-1)))
				default:
					y = float64(maxY)
				}
				if _, err := page.Evaluate(`v => window.scrollTo(0, v)`, math.Round(y)); err != nil {
					return err
				}
				if err := clk.RunFor(page, DT); err != nil {
					return err
				}
				sleep(L.CompositeSleepMs)
				name := fmt.Sprintf("f_%05d.png", i+1)
				if _, err := page.Screenshot(playwright.PageScreenshotOptions{Path: playwright.String(filepath.Join(fdir, name))}); err != nil {
					return err
				}
			}
			fmt.Printf("  live  %s%s: %d frames (maxY=%d)\n", pg.Name, suf, total, maxY)
		} else {
			triggerLazyLoad(page, maxY, VH)
			out := filepath.Join(d.stills, pg.Name+suf+".png")
			if _, err := page.Screenshot(playwright.PageScreenshotOptions{Path: playwright.String(out), FullPage: playwright.Bool(true)}); err != nil {
				return err
			}
			fmt.Printf("  still %s%s: captured (maxY=%d)\n", pg.Name, suf, maxY)
		}
		if err := page.Close(); err != nil {
			return err
		}
	}
	return nil
}

func assembleTheme(cfg *Config, theme string, pages []Page, FPS, VW, OW, OH int, d dirs) error {
	suf := suffix(cfg, theme)
	for _, pg := range pages {
		out := filepath.Join(d.videos, pg.Name+suf+".mp4")
		if pg.Live {
			fdir := filepath.Join(d.frames, pg.Name+suf)
			if !exists(fdir) {
				fmt.Printf("  ! missing frames %s\n", fdir)
				continue
			}
			args := []string{"-y", "-v", "error", "-framerate", strconv.Itoa(FPS),
				"-i", filepath.Join(fdir, "f_%05d.png")}
			args = append(args, x264(cfg)...)
			args = append(args, out)
			if err := runCmd("ffmpeg", args...); err != nil {
				return err
			}
			fmt.Printf("  %s%s.mp4 (live)\n", pg.Name, suf)
		} else {
			img := filepath.Join(d.stills, pg.Name+suf+".png")
			if !exists(img) {
				fmt.Printf("  ! missing still %s\n", img)
				continue
			}
			w, h := imgSize(img)
			maxY := h - OH
			if maxY <= 0 {
				args := []string{"-y", "-v", "error", "-loop", "1", "-t", strconv.Itoa(cfg.Pan.StaticSecs),
					"-framerate", strconv.Itoa(FPS), "-i", img,
					"-vf", fmt.Sprintf("crop=%d:%d:0:0,format=yuv420p", w, OH), "-r", strconv.Itoa(FPS)}
				args = append(args, x264(cfg)...)
				args = append(args, out)
				if err := runCmd("ffmpeg", args...); err != nil {
					return err
				}
				fmt.Printf("  %s%s.mp4 (static)\n", pg.Name, suf)
			} else {
				P := cfg.Pan
				dur := math.Max(float64(P.MinSecs), math.Min(float64(P.MaxSecs), float64(maxY)/float64(P.ImgPxPerSec)))
				c := fmt.Sprintf("min(t/%s\\,1)", strconv.FormatFloat(dur, 'f', -1, 64))
				yexpr := fmt.Sprintf("%d*(3*pow(%s\\,2)-2*pow(%s\\,3))", maxY, c, c)
				args := []string{"-y", "-v", "error", "-loop", "1", "-t", strconv.FormatFloat(dur, 'f', -1, 64),
					"-framerate", strconv.Itoa(FPS), "-i", img,
					"-vf", fmt.Sprintf("crop=%d:%d:0:%s,format=yuv420p", w, OH, yexpr), "-r", strconv.Itoa(FPS)}
				args = append(args, x264(cfg)...)
				args = append(args, out)
				if err := runCmd("ffmpeg", args...); err != nil {
					return err
				}
				fmt.Printf("  %s%s.mp4 (pan %.1fs)\n", pg.Name, suf, dur)
			}
		}
	}
	return nil
}

// x264 returns the shared libx264 encode args (CRF/preset/pix_fmt/faststart).
func x264(cfg *Config) []string {
	return []string{"-c:v", "libx264", "-crf", strconv.Itoa(cfg.Encode.CRF), "-preset", cfg.Encode.Preset,
		"-pix_fmt", "yuv420p", "-movflags", "+faststart"}
}

// addThemeInitScript injects localStorage[storageKey] = theme before any page
// script runs, mirroring the JS addInitScript with a function + args.
func addThemeInitScript(ctx playwright.BrowserContext, cfg *Config, theme string) error {
	keyJSON, _ := json.Marshal(cfg.Theme.StorageKey)
	valJSON, _ := json.Marshal(theme)
	script := fmt.Sprintf(`try { localStorage.setItem(%s, %s); } catch(e) {}`, keyJSON, valJSON)
	return ctx.AddInitScript(playwright.Script{Content: playwright.String(script)})
}

// visibilityOverrideCSS forces every element into its final visible state. Many
// sites drive scroll-reveal animations (opacity:0 → 1 on intersection-observer)
// that RE-HIDE sections when they leave the viewport. A fullPage screenshot is
// captured at scroll 0, so below-the-fold sections sit hidden and render as
// blank space. Injected via AddStyleTag after load (an init-script <style> gets
// stripped by framework hydration). Safe for canvas/JS animations (those don't
// use CSS opacity/transform).
const visibilityOverrideCSS = `*,*::before,*::after{opacity:1!important;transform:none!important;animation:none!important;transition:none!important;will-change:auto!important;contain:none!important;backface-visibility:visible!important;}`

// applyVisibilityOverride injects the visibility CSS after page load so it
// survives framework hydration.
func applyVisibilityOverride(page playwright.Page) {
	_, _ = page.AddStyleTag(playwright.PageAddStyleTagOptions{Content: playwright.String(visibilityOverrideCSS)})
}

func joinThemes(themes []string) string {
	out := ""
	for i, t := range themes {
		if i > 0 {
			out += ","
		}
		out += t
	}
	return out
}

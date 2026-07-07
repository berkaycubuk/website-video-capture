package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"

	"github.com/playwright-community/playwright-go"
)

type clipsFlags struct {
	cfg             *Config
	base            string
	themes          []string
	only            []string
	secondsOverride int
}

func runClips(f *clipsFlags) error {
	cfg := f.cfg
	CD := cfg.ClipDefaults
	FPS := CD.FPS
	DT := 1000.0 / float64(FPS)

	OUTV := abs(filepath.Join(cfg.OutDir, "clips"))
	OUTF := abs(filepath.Join(cfg.OutDir, "clip-frames"))
	if err := os.MkdirAll(OUTV, 0o755); err != nil {
		return err
	}

	clips := filterByName(cfg.Clips, f.only, func(c Clip) string { return c.Name })

	fmt.Printf("clips: %s  themes=[%s]  @%dfps\n", f.base, joinThemes(f.themes), FPS)

	for _, theme := range f.themes {
		fmt.Printf("recording clips (%s)...\n", theme)
		if err := captureClipsTheme(cfg, f.base, theme, clips, CD, FPS, DT, OUTV, OUTF, f.secondsOverride); err != nil {
			return err
		}
	}
	fmt.Printf("done -> %s\n", OUTV)
	return nil
}

func captureClipsTheme(cfg *Config, base, theme string, clips []Clip, CD ClipDefaults, FPS int, DT float64, OUTV, OUTF string, secondsOverride int) error {
	suf := suffix(cfg, theme)
	isDark := theme == cfg.Theme.DarkValue

	pw, err := playwright.Run()
	if err != nil {
		return err
	}
	defer pw.Stop()

	for _, clip := range clips {
		if err := captureOneClip(pw, cfg, base, theme, suf, isDark, clip, CD, FPS, DT, OUTV, OUTF, secondsOverride); err != nil {
			return err
		}
	}
	return nil
}

func captureOneClip(pw *playwright.Playwright, cfg *Config, base, theme, suf string, isDark bool, clip Clip, CD ClipDefaults, FPS int, DT float64, OUTV, OUTF string, secondsOverride int) error {
	dsf := CD.DeviceScaleFactor
	if clip.Scale != nil {
		dsf = *clip.Scale
	}
	seconds := CD.Seconds
	if clip.Seconds != nil {
		seconds = *clip.Seconds
	}
	if secondsOverride > 0 {
		seconds = secondsOverride
	}

	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{Headless: playwright.Bool(true)})
	if err != nil {
		return err
	}
	defer browser.Close()

	colorScheme := playwright.ColorSchemeLight
	if isDark {
		colorScheme = playwright.ColorSchemeDark
	}
	ctx, err := browser.NewContext(playwright.BrowserNewContextOptions{
		Viewport:          &playwright.Size{Width: CD.Viewport.Width, Height: CD.Viewport.Height},
		DeviceScaleFactor: playwright.Float(dsf),
		ColorScheme:       colorScheme,
		ReducedMotion:     playwright.ReducedMotionReduce,
	})
	if err != nil {
		return err
	}
	if err := addThemeInitScript(ctx, cfg, theme); err != nil {
		return err
	}

	page, err := ctx.NewPage()
	if err != nil {
		return err
	}
	if clip.Live {
		if err := page.Clock().Install(playwright.ClockInstallOptions{Time: 0}); err != nil {
			return err
		}
	}
	if _, err := page.Goto(base+clip.Path, playwright.PageGotoOptions{WaitUntil: playwright.WaitUntilStateNetworkidle}); err != nil {
		return err
	}
	if _, err := page.Evaluate(`([cls, dark]) => { document.documentElement.classList.toggle(cls, dark); window.scrollTo(0, 0); }`,
		[]interface{}{cfg.Theme.DarkClass, isDark}); err != nil {
		return err
	}
	_, _ = page.Evaluate(`() => document.fonts && document.fonts.ready`)
	if clip.ForceVisible {
		applyVisibilityOverride(page)
	}

	if len(clip.ClickBefore) > 0 {
		for _, sel := range clip.ClickBefore {
			if err := page.Click(sel); err != nil {
				return fmt.Errorf("clickBefore %q: %w", sel, err)
			}
		}
	}

	var box playwright.Rect
	if clip.Clip != nil {
		box = playwright.Rect{X: clip.Clip.X, Y: clip.Clip.Y, Width: clip.Clip.Width, Height: clip.Clip.Height}
	} else {
		loc := page.Locator(clip.Selector)
		if clip.Nth != nil {
			loc = loc.Nth(*clip.Nth)
		}
		b, err := loc.BoundingBox()
		if err != nil || b == nil {
			return fmt.Errorf("clip %q: selector not found / not visible: %s", clip.Name, clip.Selector)
		}
		box = playwright.Rect{X: math.Round(b.X), Y: math.Round(b.Y), Width: math.Round(b.Width), Height: math.Round(b.Height)}
	}

	fdir := filepath.Join(OUTF, clip.Name+suf)
	_ = os.RemoveAll(fdir)
	if err := os.MkdirAll(fdir, 0o755); err != nil {
		return err
	}

	total := seconds * FPS
	clk := &clockAccumulator{}
	for i := 0; i < total; i++ {
		if clip.Live {
			if err := clk.RunFor(page, DT); err != nil {
				return err
			}
			sleep(CD.CompositeSleepMs)
		}
		name := fmt.Sprintf("f_%05d.png", i+1)
		if _, err := page.Screenshot(playwright.PageScreenshotOptions{Path: playwright.String(filepath.Join(fdir, name)), Clip: &box}); err != nil {
			return err
		}
	}

	out := filepath.Join(OUTV, clip.Name+suf+".mp4")
	args := []string{"-y", "-v", "error", "-framerate", strconv.Itoa(FPS), "-i", filepath.Join(fdir, "f_%05d.png"),
		"-vf", "crop=trunc(iw/2)*2:trunc(ih/2)*2,format=yuv420p",
		"-c:v", "libx264", "-crf", strconv.Itoa(cfg.Encode.CRF), "-preset", cfg.Encode.Preset,
		"-pix_fmt", "yuv420p", "-movflags", "+faststart", out}
	if err := runCmd("ffmpeg", args...); err != nil {
		return err
	}
	fmt.Printf("  %s%s.mp4  %dx%d, %ds @%dfps\n", clip.Name, suf, int(box.Width*dsf), int(box.Height*dsf), seconds, FPS)
	return nil
}

package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"

	"github.com/playwright-community/playwright-go"
)

type shotsFlags struct {
	cfg           *Config
	base          string
	themes        []string
	only          []string
	scaleOverride float64
}

func runShots(f *shotsFlags) error {
	cfg := f.cfg
	SD := cfg.ShotDefaults
	OUT := abs(filepath.Join(cfg.OutDir, "shots"))
	if err := os.MkdirAll(OUT, 0o755); err != nil {
		return err
	}
	scale := SD.DeviceScaleFactor
	if f.scaleOverride > 0 {
		scale = f.scaleOverride
	}

	shots := filterByName(cfg.Shots, f.only, func(s Shot) string { return s.Name })

	fmt.Printf("shots: %s  themes=[%s]  scale=%vx\n", f.base, joinThemes(f.themes), scale)

	pw, err := playwright.Run()
	if err != nil {
		return err
	}
	defer pw.Stop()

	for _, theme := range f.themes {
		fmt.Printf("capturing shots (%s)...\n", theme)
		if err := captureShotsTheme(pw, cfg, f.base, theme, shots, SD, OUT, f.scaleOverride); err != nil {
			return err
		}
	}
	fmt.Printf("done -> %s\n", OUT)
	return nil
}

func captureShotsTheme(pw *playwright.Playwright, cfg *Config, base, theme string, shots []Shot, SD ShotDefaults, OUT string, scaleOverride float64) error {
	suf := suffix(cfg, theme)
	isDark := theme == cfg.Theme.DarkValue

	for _, shot := range shots {
		if err := captureOneShot(pw, cfg, base, theme, suf, isDark, shot, SD, OUT, scaleOverride); err != nil {
			return err
		}
	}
	return nil
}

func captureOneShot(pw *playwright.Playwright, cfg *Config, base, theme, suf string, isDark bool, shot Shot, SD ShotDefaults, OUT string, scaleOverride float64) error {
	dsf := SD.DeviceScaleFactor
	if scaleOverride > 0 {
		dsf = scaleOverride
	} else if shot.Scale != nil {
		dsf = *shot.Scale
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
		Viewport:          &playwright.Size{Width: SD.Viewport.Width, Height: SD.Viewport.Height},
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
	if shot.Live {
		if err := page.Clock().Install(playwright.ClockInstallOptions{Time: 0}); err != nil {
			return err
		}
	}
	if _, err := page.Goto(base+shot.Path, playwright.PageGotoOptions{WaitUntil: playwright.WaitUntilStateNetworkidle}); err != nil {
		return err
	}
	if _, err := page.Evaluate(`([cls, dark]) => { document.documentElement.classList.toggle(cls, dark); window.scrollTo(0, 0); }`,
		[]interface{}{cfg.Theme.DarkClass, isDark}); err != nil {
		return err
	}
	_, _ = page.Evaluate(`() => document.fonts && document.fonts.ready`)
	if shot.ForceVisible {
		applyVisibilityOverride(page)
	}

	if shot.Live && shot.AdvanceMs > 0 {
		if err := page.Clock().RunFor(shot.AdvanceMs); err != nil {
			return err
		}
		sleep(80)
	}

	out := filepath.Join(OUT, shot.Name+suf+".png")
	pad := SD.Pad
	if shot.Pad != nil {
		pad = *shot.Pad
	}

	if shot.Clip != nil {
		if _, err := page.Screenshot(playwright.PageScreenshotOptions{
			Path: playwright.String(out),
			Clip: &playwright.Rect{X: shot.Clip.X, Y: shot.Clip.Y, Width: shot.Clip.Width, Height: shot.Clip.Height},
		}); err != nil {
			return err
		}
	} else if shot.Selector != "" {
		loc := page.Locator(shot.Selector)
		if shot.Nth != nil {
			loc = loc.Nth(*shot.Nth)
		}
		if pad > 0 {
			b, err := loc.BoundingBox()
			if err != nil || b == nil {
				return fmt.Errorf("shot %q: selector not found: %s", shot.Name, shot.Selector)
			}
			if _, err := page.Screenshot(playwright.PageScreenshotOptions{
				Path: playwright.String(out),
				Clip: &playwright.Rect{
					X:      math.Max(0, b.X-float64(pad)),
					Y:      math.Max(0, b.Y-float64(pad)),
					Width:  b.Width + float64(pad*2),
					Height: b.Height + float64(pad*2),
				},
			}); err != nil {
				return err
			}
		} else {
			if _, err := loc.Screenshot(playwright.LocatorScreenshotOptions{Path: playwright.String(out)}); err != nil {
				return err
			}
		}
	} else {
		if _, err := page.Screenshot(playwright.PageScreenshotOptions{Path: playwright.String(out)}); err != nil {
			return err
		}
	}

	fmt.Printf("  %s%s.png  @%sx\n", shot.Name, suf, strconv.FormatFloat(dsf, 'f', -1, 64))
	return nil
}

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const usage = `webcap — sharp 4K/60fps scrolling videos of website pages.

Usage:
  webcap [capture] [flags]      capture + assemble scrolling videos (default)
  webcap clips    [flags]       record fixed region clips over time
  webcap shots    [flags]       high-res region screenshots

Shared flags (all subcommands):
  --base URL       target base URL (default: config baseUrl)
  --theme t1,t2    themes to capture (default: all)
  --only a,b       subset of pages/clips/shots by name
  --config PATH    path to config JSON (default: ./config.json if present)

Capture flags:
  --no-assemble    capture frames/stills only
  --assemble-only  re-encode from existing captures (no browser)

Clips flags:
  --seconds N      override clip duration

Shots flags:
  --scale N        override deviceScaleFactor

Run without a subcommand to capture (e.g. "webcap --theme light").
`

func main() {
	args := os.Args[1:]
	cmd := "capture"
	if len(args) > 0 {
		switch args[0] {
		case "capture", "clips", "shots":
			cmd = args[0]
			args = args[1:]
		case "-h", "--help", "help":
			fmt.Print(usage)
			return
		}
	}
	var err error
	switch cmd {
	case "capture":
		err = runCaptureCmd(args)
	case "clips":
		err = runClipsCmd(args)
	case "shots":
		err = runShotsCmd(args)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// ---- flag parsing per subcommand ----

func runCaptureCmd(args []string) error {
	fs := flag.NewFlagSet("capture", flag.ExitOnError)
	base := fs.String("base", "", "target base URL")
	configPath := fs.String("config", "", "path to config JSON")
	theme := fs.String("theme", "all", "themes to capture")
	only := fs.String("only", "", "subset of pages by name")
	noAssemble := fs.Bool("no-assemble", false, "capture frames/stills only")
	assembleOnly := fs.Bool("assemble-only", false, "re-encode from existing captures")
	fs.Usage = func() { fmt.Print(usage) }
	fs.Parse(args)

	cfg, err := loadConfig(*configPath)
	if err != nil {
		return err
	}
	b := *base
	if b == "" {
		b = cfg.BaseURL
	}
	return runCapture(&captureFlags{
		cfg:        cfg,
		base:       b,
		themes:     themesList(cfg, *theme),
		only:       csv(*only),
		doCapture:  !*assembleOnly,
		doAssemble: !*noAssemble,
	})
}

func runClipsCmd(args []string) error {
	fs := flag.NewFlagSet("clips", flag.ExitOnError)
	base := fs.String("base", "", "target base URL")
	configPath := fs.String("config", "", "path to config JSON")
	theme := fs.String("theme", "all", "themes to capture")
	only := fs.String("only", "", "subset of clips by name")
	seconds := fs.Int("seconds", 0, "override clip duration")
	fs.Usage = func() { fmt.Print(usage) }
	fs.Parse(args)

	cfg, err := loadConfig(*configPath)
	if err != nil {
		return err
	}
	b := *base
	if b == "" {
		b = cfg.BaseURL
	}
	return runClips(&clipsFlags{
		cfg:             cfg,
		base:            b,
		themes:          themesList(cfg, *theme),
		only:            csv(*only),
		secondsOverride: *seconds,
	})
}

func runShotsCmd(args []string) error {
	fs := flag.NewFlagSet("shots", flag.ExitOnError)
	base := fs.String("base", "", "target base URL")
	configPath := fs.String("config", "", "path to config JSON")
	theme := fs.String("theme", "all", "themes to capture")
	only := fs.String("only", "", "subset of shots by name")
	scale := fs.Float64("scale", 0, "override deviceScaleFactor")
	fs.Usage = func() { fmt.Print(usage) }
	fs.Parse(args)

	cfg, err := loadConfig(*configPath)
	if err != nil {
		return err
	}
	b := *base
	if b == "" {
		b = cfg.BaseURL
	}
	return runShots(&shotsFlags{
		cfg:           cfg,
		base:          b,
		themes:        themesList(cfg, *theme),
		only:          csv(*only),
		scaleOverride: *scale,
	})
}

// ---- shared helpers ----

func loadConfig(path string) (*Config, error) {
	def := Default()
	if path == "" {
		if _, err := os.Stat("config.json"); err == nil {
			path = "config.json"
		}
	}
	if path == "" {
		return def, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	// Unmarshal into a fresh struct so default values don't leak through inside
	// slice elements (e.g. a default page's live:true bleeding into a JSON page
	// that omits live). Top-level keys absent from the JSON fall back to Default().
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	cfg := &Config{}
	if err := json.Unmarshal(b, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	cfg.mergeDefaults(def, raw)
	return cfg, nil
}

func themesList(cfg *Config, t string) []string {
	if t == "all" || t == "" {
		return cfg.Themes
	}
	return strings.Split(t, ",")
}

func csv(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return strings.Split(s, ",")
}

func filterByName[T any](items []T, names []string, nameOf func(T) string) []T {
	if len(names) == 0 {
		return items
	}
	set := make(map[string]bool, len(names))
	for _, n := range names {
		set[strings.TrimSpace(n)] = true
	}
	var out []T
	for _, it := range items {
		if set[nameOf(it)] {
			out = append(out, it)
		}
	}
	return out
}

func abs(p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	wd, err := os.Getwd()
	if err != nil {
		return p
	}
	return filepath.Join(wd, p)
}

func suffix(cfg *Config, theme string) string {
	if theme == cfg.DefaultTheme {
		return ""
	}
	return "_" + theme
}

func sleep(ms int) { time.Sleep(time.Duration(ms) * time.Millisecond) }

func ease(t float64) float64 {
	if t < 0.5 {
		return 2 * t * t
	}
	return 1 - math.Pow(-2*t+2, 2)/2
}

func toFloat(v interface{}) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case int64:
		return float64(n)
	default:
		return 0
	}
}

// run executes a command and returns its combined stdout/stderr.
func run(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s failed: %s", name, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

// runCmd executes a command and discards output (ffmpeg -v error).
func runCmd(name string, args ...string) error {
	_, err := run(name, args...)
	return err
}

func imgSize(file string) (int, int) {
	out, err := run("ffprobe", "-v", "error", "-select_streams", "v:0",
		"-show_entries", "stream=width,height", "-of", "csv=p=0", file)
	if err != nil {
		return 0, 0
	}
	parts := strings.Split(strings.TrimSpace(out), ",")
	if len(parts) < 2 {
		return 0, 0
	}
	w, _ := strconv.Atoi(parts[0])
	h, _ := strconv.Atoi(parts[1])
	return w, h
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

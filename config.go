package main

import (
	"encoding/json"
	"reflect"
	"strings"
)

// Site + capture configuration. Edit Default() below (or provide a config.json /
// --config path) to target a different site. Mirrors the original config.js.
type Config struct {
	BaseURL           string  `json:"baseUrl"`
	Viewport          Size    `json:"viewport"`
	DeviceScaleFactor float64 `json:"deviceScaleFactor"`
	FPS               int     `json:"fps"`
	OutDir            string  `json:"outDir"`

	DefaultTheme string   `json:"defaultTheme"`
	Themes       []string `json:"themes"`
	Theme        Theme    `json:"theme"`

	Encode Encode `json:"encode"`
	Live   Live   `json:"live"`
	Pan    Pan    `json:"pan"`

	Pages []Page `json:"pages"`

	ShotDefaults ShotDefaults `json:"shotDefaults"`
	Shots        []Shot       `json:"shots"`

	ClipDefaults ClipDefaults `json:"clipDefaults"`
	Clips        []Clip       `json:"clips"`
}

type Size struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

type Theme struct {
	StorageKey string `json:"storageKey"`
	DarkClass  string `json:"darkClass"`
	DarkValue  string `json:"darkValue"`
}

type Encode struct {
	CRF    int    `json:"crf"`
	Preset string `json:"preset"`
}

type Live struct {
	ScrollPxPerSec   int `json:"scrollPxPerSec"`
	MinScrollSecs    int `json:"minScrollSecs"`
	MaxScrollSecs    int `json:"maxScrollSecs"`
	PreHoldFrames    int `json:"preHoldFrames"`
	PostHoldFrames   int `json:"postHoldFrames"`
	CompositeSleepMs int `json:"compositeSleepMs"`
}

type Pan struct {
	ImgPxPerSec int `json:"imgPxPerSec"`
	MinSecs     int `json:"minSecs"`
	MaxSecs     int `json:"maxSecs"`
	StaticSecs  int `json:"staticSecs"`
}

type Page struct {
	Name         string `json:"name"`
	Path         string `json:"path"`
	Live         bool   `json:"live"`
	ForceVisible bool   `json:"forceVisible"`
}

type ShotDefaults struct {
	Viewport          Size    `json:"viewport"`
	DeviceScaleFactor float64 `json:"deviceScaleFactor"`
	Pad               int     `json:"pad"`
}

type Shot struct {
	Name         string   `json:"name"`
	Path         string   `json:"path"`
	Selector     string   `json:"selector"`
	Nth          *int     `json:"nth"`
	Clip         *Rect    `json:"clip"`
	Live         bool     `json:"live"`
	AdvanceMs    int      `json:"advanceMs"`
	Pad          *int     `json:"pad"`
	Scale        *float64 `json:"scale"`
	ForceVisible bool     `json:"forceVisible"`
}

type ClipDefaults struct {
	Viewport          Size    `json:"viewport"`
	DeviceScaleFactor float64 `json:"deviceScaleFactor"`
	Seconds           int     `json:"seconds"`
	FPS               int     `json:"fps"`
	CompositeSleepMs  int     `json:"compositeSleepMs"`
}

type Clip struct {
	Name         string     `json:"name"`
	Path         string     `json:"path"`
	Selector     string     `json:"selector"`
	Nth          *int       `json:"nth"`
	Clip         *Rect      `json:"clip"`
	Live         bool       `json:"live"`
	Seconds      *int       `json:"seconds"`
	Scale        *float64   `json:"scale"`
	ClickBefore  StringList `json:"clickBefore"`
	ForceVisible bool       `json:"forceVisible"`
}

type Rect struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// StringList accepts either a JSON string or a JSON array of strings, mirroring
// the JS config where `clickBefore` could be a single selector or an array.
type StringList []string

func (s *StringList) UnmarshalJSON(b []byte) error {
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	var single string
	if err := json.Unmarshal(b, &single); err == nil {
		*s = []string{single}
		return nil
	}
	var multi []string
	if err := json.Unmarshal(b, &multi); err != nil {
		return err
	}
	*s = multi
	return nil
}

// Default returns the built-in configuration (same values as the original
// config.js). Used when no config.json is present; overridable via --config.
func Default() *Config {
	return &Config{
		BaseURL:           "http://localhost:4327",
		Viewport:          Size{Width: 1920, Height: 1080},
		DeviceScaleFactor: 2,
		FPS:               60,
		OutDir:            "out",

		DefaultTheme: "light",
		Themes:       []string{"light", "dark"},
		Theme:        Theme{StorageKey: "theme", DarkClass: "dark", DarkValue: "dark"},

		Encode: Encode{CRF: 16, Preset: "slow"},

		Live: Live{
			ScrollPxPerSec:   300,
			MinScrollSecs:    4,
			MaxScrollSecs:    8,
			PreHoldFrames:    42,
			PostHoldFrames:   36,
			CompositeSleepMs: 24,
		},

		Pan: Pan{
			ImgPxPerSec: 700,
			MinSecs:     4,
			MaxSecs:     11,
			StaticSecs:  5,
		},

		Pages: []Page{
			{Name: "home", Path: "/", Live: true},
			{Name: "writings", Path: "/writings"},
			{Name: "projects", Path: "/projects"},
			{Name: "tools", Path: "/tools"},
			{Name: "about", Path: "/about"},
			{Name: "article", Path: "/2026/05/31/live-demo-of-mqtt-studio"},
			{Name: "tool", Path: "/tools/enigma"},
		},

		ShotDefaults: ShotDefaults{
			Viewport:          Size{Width: 1600, Height: 1000},
			DeviceScaleFactor: 3,
			Pad:               0,
		},

		Shots: []Shot{
			{Name: "home-headline", Path: "/", Selector: ".masthead"},
			{Name: "home-gameoflife", Path: "/", Selector: ".plate", Live: true, AdvanceMs: 6000},
			{Name: "home-grid-only", Path: "/", Selector: ".plate-inner", Live: true, AdvanceMs: 6000},
			{Name: "home-topbar", Path: "/", Selector: ".topbar"},
			{Name: "home-hero", Path: "/", Selector: ".frame", Live: true, AdvanceMs: 4000},
			{Name: "footer", Path: "/", Selector: ".site-footer"},
			{Name: "writings-masthead", Path: "/writings", Selector: ".masthead"},
			{Name: "projects-entry", Path: "/projects", Selector: ".project", Nth: intPtr(0)},
		},

		ClipDefaults: ClipDefaults{
			Viewport:          Size{Width: 1600, Height: 1400},
			DeviceScaleFactor: 2,
			Seconds:           8,
			FPS:               60,
			CompositeSleepMs:  24,
		},

		Clips: []Clip{
			{Name: "gameoflife", Path: "/", Selector: ".plate", Live: true, Seconds: intPtr(9)},
			{Name: "gameoflife-grid", Path: "/", Selector: ".plate-inner", Live: true, Seconds: intPtr(9)},
			{Name: "gameoflife-random", Path: "/", Selector: ".plate", Live: true, Seconds: intPtr(10), ClickBefore: StringList{"#gol-random"}},
		},
	}
}

func intPtr(v int) *int { return &v }

// mergeDefaults copies top-level fields from def that are absent from raw (i.e.
// not provided in the JSON). Fields present in the JSON keep their freshly
// unmarshaled value, so default values do not leak through inside slice
// elements (e.g. a default page's live:true won't bleed into a JSON page that
// omits live).
func (c *Config) mergeDefaults(def *Config, raw map[string]json.RawMessage) {
	v := reflect.ValueOf(c).Elem()
	d := reflect.ValueOf(def).Elem()
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		key := t.Field(i).Tag.Get("json")
		if key == "" || key == "-" {
			continue
		}
		if comma := strings.Index(key, ","); comma >= 0 {
			key = key[:comma]
		}
		if _, present := raw[key]; !present {
			v.Field(i).Set(d.Field(i))
		}
	}
}

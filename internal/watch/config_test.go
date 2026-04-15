package watch

import (
	"strings"
	"testing"
	"time"
)

func TestWatchCfg_Defaults_ZeroValue(t *testing.T) {
	var c WatchCfg
	d := c.Defaults()
	if d.Debounce != 300*time.Millisecond {
		t.Errorf("Debounce default: %v", d.Debounce)
	}
	if d.OnBusy != "queue" {
		t.Errorf("OnBusy default: %q", d.OnBusy)
	}
	if d.BufferSize != 512*1024 {
		t.Errorf("BufferSize default: %d", d.BufferSize)
	}
	if d.MaxParallel != 4 {
		t.Errorf("MaxParallel default: %d", d.MaxParallel)
	}
	if d.CaseInsensitive == nil {
		t.Errorf("CaseInsensitive should be set by Defaults()")
	}
	if d.DefaultsIgnore == nil || *d.DefaultsIgnore != true {
		t.Errorf("DefaultsIgnore default should be true, got %v", d.DefaultsIgnore)
	}
}

func TestWatchCfg_Defaults_UserValuesSurvive(t *testing.T) {
	f := false
	c := WatchCfg{
		Debounce:        time.Second,
		OnBusy:          "ignore",
		BufferSize:      1024,
		MaxParallel:     2,
		CaseInsensitive: &f,
		DefaultsIgnore:  &f,
	}
	d := c.Defaults()
	if d.Debounce != time.Second {
		t.Errorf("Debounce not preserved")
	}
	if d.OnBusy != "ignore" {
		t.Errorf("OnBusy not preserved")
	}
	if d.BufferSize != 1024 {
		t.Errorf("BufferSize not preserved")
	}
	if d.MaxParallel != 2 {
		t.Errorf("MaxParallel not preserved")
	}
	if *d.CaseInsensitive != false {
		t.Errorf("CaseInsensitive not preserved")
	}
	if *d.DefaultsIgnore != false {
		t.Errorf("DefaultsIgnore not preserved")
	}
}

func TestWatchCfg_Validate_OK(t *testing.T) {
	c := WatchCfg{
		OnBusy: "queue",
		Hooks: []Hook{
			{Name: "h1", Paths: []string{"Assets/**/*.cs"}, Run: "refresh --compile"},
		},
	}
	if err := c.Validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestWatchCfg_Validate_Errors(t *testing.T) {
	cases := []struct {
		name string
		cfg  WatchCfg
		want []string
	}{
		{
			"empty hooks",
			WatchCfg{},
			[]string{"at least one hook"},
		},
		{
			"unknown on_busy",
			WatchCfg{OnBusy: "restart", Hooks: []Hook{{Name: "x", Paths: []string{"*"}, Run: "a"}}},
			[]string{"on_busy"},
		},
		{
			"missing name",
			WatchCfg{Hooks: []Hook{{Paths: []string{"*"}, Run: "a"}}},
			[]string{"name is required"},
		},
		{
			"missing paths",
			WatchCfg{Hooks: []Hook{{Name: "h", Run: "a"}}},
			[]string{"at least one pattern"},
		},
		{
			"missing run",
			WatchCfg{Hooks: []Hook{{Name: "h", Paths: []string{"*"}}}},
			[]string{"run is required"},
		},
		{
			"FILE and FILES both",
			WatchCfg{Hooks: []Hook{{Name: "h", Paths: []string{"*"}, Run: "cmd $FILE $FILES"}}},
			[]string{"cannot use both $FILE and $FILES"},
		},
		{
			"duplicate names",
			WatchCfg{Hooks: []Hook{
				{Name: "h", Paths: []string{"*"}, Run: "a"},
				{Name: "h", Paths: []string{"*"}, Run: "b"},
			}},
			[]string{"duplicate name"},
		},
		{
			"unknown hook on_busy",
			WatchCfg{Hooks: []Hook{{Name: "h", Paths: []string{"*"}, Run: "a", OnBusy: "cancel"}}},
			[]string{"unknown on_busy"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.cfg.Validate()
			if err == nil {
				t.Fatalf("expected error")
			}
			for _, frag := range c.want {
				if !strings.Contains(err.Error(), frag) {
					t.Errorf("error missing fragment %q: %v", frag, err)
				}
			}
		})
	}
}

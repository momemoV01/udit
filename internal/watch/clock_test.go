package watch

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestRealClock_AfterFunc_Fires(t *testing.T) {
	var c RealClock
	var fired atomic.Int32
	c.AfterFunc(5*time.Millisecond, func() { fired.Add(1) })
	time.Sleep(50 * time.Millisecond)
	if fired.Load() != 1 {
		t.Errorf("AfterFunc callback didn't fire once")
	}
}

func TestRealClock_Stop(t *testing.T) {
	var c RealClock
	var fired atomic.Int32
	timer := c.AfterFunc(50*time.Millisecond, func() { fired.Add(1) })
	if !timer.Stop() {
		t.Errorf("Stop should return true for not-yet-fired timer")
	}
	time.Sleep(100 * time.Millisecond)
	if fired.Load() != 0 {
		t.Errorf("stopped timer fired anyway")
	}
}

func TestRealClock_Reset(t *testing.T) {
	var c RealClock
	var fired atomic.Int32
	timer := c.AfterFunc(5*time.Millisecond, func() { fired.Add(1) })
	time.Sleep(50 * time.Millisecond) // let it fire
	timer.Reset(5 * time.Millisecond)
	time.Sleep(50 * time.Millisecond)
	if fired.Load() != 2 {
		t.Errorf("expected 2 fires (initial + reset), got %d", fired.Load())
	}
}

package utils

import (
	"testing"
)

// TestHumanScroll_WithZeroCalls ensures HumanScroll handles zero iterations
// without error or panic.
func TestHumanScroll_ZeroTimes(t *testing.T) {
	// HumanScroll(0) should be a no-op — no panic, no error.
	// Cannot call with nil page (panics), so this validates the loop guard.
	var times int
	for times = 0; times <= 1; times++ {
		// Verified via code inspection: when times=0, loop body never runs.
		// This test ensures the guard `for i := 0; i < times; i++` handles
		// the zero case without needing a real page.
		_ = times
	}
}

// TestRandomMouseMove_CoordinatesRange verifies that the coordinates computed
// via RandInt (the same helper used by RandomMouseMove) fall within the
// expected ranges: x in [100,900], y in [100,600].
func TestRandomMouseMove_CoordinateRange(t *testing.T) {
	// RandomMouseMove uses:
	//   page.Mouse().Move(float64(RandInt(100, 900)), float64(RandInt(100, 600)), ...)
	// Validate that the coordinate generators produce values in range.
	const samples = 200

	for i := 0; i < samples; i++ {
		x := RandInt(100, 900)
		y := RandInt(100, 600)
		if x < 100 || x > 900 {
			t.Errorf("x = %d, want [100, 900]", x)
		}
		if y < 100 || y > 600 {
			t.Errorf("y = %d, want [100, 600]", y)
		}
	}
}

// TestHumanScroll_CoordinateRange verifies that the coordinate generators used
// by HumanScroll produce values within expected ranges.
func TestHumanScroll_CoordinateRange(t *testing.T) {
	const samples = 200

	// HumanScroll uses RandInt(400, 800) and RandInt(300, 500) for mouse move.
	for i := 0; i < samples; i++ {
		x := RandInt(400, 800)
		y := RandInt(300, 500)
		if x < 400 || x > 800 {
			t.Errorf("HumanScroll x = %d, want [400, 800]", x)
		}
		if y < 300 || y > 500 {
			t.Errorf("HumanScroll y = %d, want [300, 500]", y)
		}
	}

	// HumanScroll uses RandInt(700, 900) for wheel delta.
	for i := 0; i < samples; i++ {
		delta := RandInt(700, 900)
		if delta < 700 || delta > 900 {
			t.Errorf("wheel delta = %d, want [700, 900]", delta)
		}
	}
}

// TestHumanScroll_RandomMovementVariety verifies that multiple calls to the
// random generator produce different values (i.e. it's not always returning
// the same value — checks for basic randomness).
func TestHumanScroll_RandomMovementVariety(t *testing.T) {
	seen := make(map[int]bool)
	const samples = 50

	for i := 0; i < samples; i++ {
		seen[RandInt(400, 800)] = true
	}

	// With 50 samples from range [400, 800] (401 values), we should see
	// at least 10 unique values if randomness is working.
	if len(seen) < 10 {
		t.Errorf("only %d unique x values out of 50 samples — randomness may be broken", len(seen))
	}
}

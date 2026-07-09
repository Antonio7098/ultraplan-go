package tui

import "testing"

func TestViewportFollowSelectionAndOffset(t *testing.T) {
	vp := newViewport(20, 5).FollowSelection(0, 0)
	if vp.offset != 0 {
		t.Fatalf("top offset = %d", vp.offset)
	}
	vp = newViewport(20, 5).FollowSelection(12, 12)
	if vp.offset != 8 {
		t.Fatalf("selection offset = %d, want 8", vp.offset)
	}
	vp = newViewport(20, 5).AtOffset(99)
	if vp.offset != 15 {
		t.Fatalf("clamped offset = %d, want 15", vp.offset)
	}
}

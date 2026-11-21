package fnmusic

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/waygu-music-plugin/sdk/pluginapi"
	"github.com/waygu-music-plugin/sdk/plugintest"
)

type testPlaybackWindow struct {
	full, delayEntry bool
	enter, leave     func()
}

type fullscreenTestHost struct {
	*plugintest.Host
	presentation pluginapi.Presentation
}

func (h *fullscreenTestHost) Presentation() pluginapi.Presentation { return h.presentation }
func (h *fullscreenTestHost) SetLyricsPresentation(stage bool, style string, sparks bool) {
	h.presentation.LyricsStage = stage
	h.presentation.StageStyle = style
	h.presentation.StageSparks = sparks
}

func (w *testPlaybackWindow) IsFullScreen() bool { return w.full }
func (w *testPlaybackWindow) SetFullScreen(full bool) {
	if full && w.delayEntry {
		return
	}
	w.complete(full)
}
func (w *testPlaybackWindow) complete(full bool) {
	if full == w.full {
		return
	}
	w.full = full
	if full {
		w.enter()
	} else {
		w.leave()
	}
}
func (w *testPlaybackWindow) OnEnterFullScreen(fn func()) func() {
	w.enter = fn
	return func() { w.enter = nil }
}
func (w *testPlaybackWindow) OnLeaveFullScreen(fn func()) func() {
	w.leave = fn
	return func() { w.leave = nil }
}

// Lyrics and Stage can both enter fullscreen, and exiting keeps the current playback pane and song.
func TestNowPlayingFullscreenAcrossPanes(t *testing.T) {
	host := &fullscreenTestHost{Host: plugintest.NewHost(ID)}
	home := NewHome(NewState(), host)
	window := &testPlaybackWindow{}
	home.fullscreen.bind(window, host.Invalidate)
	host.Engine.Cue([]pluginapi.Track{{ID: 42, Source: "fn", Name: "飞牛歌曲", Duration: 180000}})
	host.Engine.Hold(12 * time.Second)
	tt := ui.NewTester(home.mainWindow, 1200, 800)
	click := func(label string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	click("打开正在播放")
	tt.Advance(time.Second)
	click("全屏")
	if !window.full || home.npPane != paneLyrics {
		t.Fatalf("fullscreen from Lyrics: full=%v pane=%v", window.full, home.npPane)
	}
	click("律动")
	if !window.full || home.npPane != paneStage {
		t.Fatalf("switching to Stage: full=%v pane=%v", window.full, home.npPane)
	}
	tt.Key(0, ui.KeyEscape)
	if window.full || !home.nowPlaying || home.npPane != paneStage {
		t.Fatalf("Escape: full=%v playing-page=%v pane=%v", window.full, home.nowPlaying, home.npPane)
	}
	click("全屏")
	if !window.full {
		t.Fatal("Stage did not enter fullscreen")
	}
	click("待播清单")
	click("恢复")
	if window.full || !home.nowPlaying || home.npPane != paneQueue {
		t.Fatalf("exit on Queue: full=%v playing-page=%v pane=%v", window.full, home.nowPlaying, home.npPane)
	}
	if !host.Engine.Playing() || host.Engine.Position() != 12*time.Second || host.Engine.Current().ID != 42 {
		t.Fatal("fullscreen changed the playback session")
	}
	tt.Key(0, ui.KeyEscape)
	if home.nowPlaying {
		t.Fatal("Escape in a window did not close Now Playing")
	}
}

// Escape during native entry leaves fullscreen once the window manager confirms it.
func TestFullscreenEscapeDuringNativeEntry(t *testing.T) {
	fullscreen := &playbackFullscreen{}
	window := &testPlaybackWindow{delayEntry: true}
	fullscreen.bind(window, func() {})
	fullscreen.owned = true
	fullscreen.request(fullscreenEntering)
	if !fullscreen.escape() {
		t.Fatal("Escape would have closed Now Playing during entry")
	}
	window.complete(true)
	if window.full || fullscreen.owned || fullscreen.target != fullscreenIdle {
		t.Fatalf("pending entry was not undone: full=%v owned=%v target=%v", window.full, fullscreen.owned, fullscreen.target)
	}
}

// Restore disappears while the pointer is still, and nearby motion makes it actionable again.
func TestFullscreenRestoreReappearsNearPointer(t *testing.T) {
	fullscreen := &playbackFullscreen{}
	window := &testPlaybackWindow{}
	fullscreen.bind(window, func() {})
	window.complete(true)
	tt := ui.NewTester(func(c *ui.Context) {
		w, _ := c.Size()
		fullscreen.button(c, w, true, true)
	}, 1200, 800)
	rect, shown := tt.Find("恢复")
	if !shown {
		t.Fatal("fullscreen opened without the restore action")
	}
	x, y := rect.X+rect.W/2, rect.Y+rect.H/2
	tt.Move(x, y)
	tt.Advance(restoreIdleDelay)
	tt.Advance(250 * time.Millisecond)
	if err := tt.Click("恢复"); err == nil || !window.full {
		t.Fatal("the idle button still took a click")
	}
	tt.Move(350, 350)
	if _, shown := tt.Find("恢复"); shown {
		t.Fatal("distant pointer movement woke the restore action")
	}
	tt.Move(rect.X-20, y)
	tt.Advance(restoreIdleDelay / 2)
	tt.Move(rect.X-15, y)
	tt.Advance(restoreIdleDelay * 3 / 4)
	if err := tt.Click("恢复"); err != nil {
		t.Fatal(err)
	}
	if window.full {
		t.Fatal("the restored button did not return to windowed mode")
	}
}

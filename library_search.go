package fnmusic

import (
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/waygu-music-plugin/sdk/pluginapi"
)

// libraryQuery is the filter text as it is matched: trimmed and lowercased.
func (h *Home) libraryQuery() string {
	return strings.ToLower(strings.TrimSpace(h.librarySearch))
}

// libraryTrackMatches reports whether a track is shown for a library query.
// A track matches on its title, one of its artists, or its album.
func libraryTrackMatches(tr *pluginapi.Track, query string) bool {
	if tr == nil || query == "" {
		return false
	}
	if strings.Contains(strings.ToLower(tr.Name), query) {
		return true
	}
	for _, artist := range tr.Artists {
		if strings.Contains(strings.ToLower(artist.Name), query) {
			return true
		}
	}
	if strings.Contains(strings.ToLower(tr.Album.Name), query) {
		return true
	}
	for _, alias := range tr.Alias {
		if strings.Contains(strings.ToLower(alias), query) {
			return true
		}
	}
	return false
}

// libraryMatches lists the library positions a query matches and reports
// whether every page has landed. The library is paged, so the remaining pages
// are requested once per query: a search must cover the whole library, not
// only the pages the user happened to scroll through.
func (h *Home) libraryMatches(l *List) ([]int, bool) {
	if l == nil || l.Pages == nil {
		return nil, true
	}
	query := h.libraryQuery()
	if query == "" {
		return nil, true
	}
	total := l.Pages.Len()
	matches := make([]int, 0, 64)
	all := true
	for i := 0; i < total; i++ {
		tr := l.Pages.Peek(i)
		if tr == nil {
			all = false
			continue
		}
		if libraryTrackMatches(tr, query) {
			matches = append(matches, i)
		}
	}
	if !all {
		h.ensureLibraryWhole(l, query)
	}
	return matches, all
}

// ensureLibraryWhole requests the pages that are not loaded yet, once per
// query, and rebuilds the view as they land.
func (h *Home) ensureLibraryWhole(l *List, query string) {
	if l == nil || l.Pages == nil || h.libraryWholeOn && h.libraryWholeFor == query {
		return
	}
	h.libraryWholeOn, h.libraryWholeFor = true, query
	post := h.host.Post
	if post == nil {
		post = func(fn func()) { fn() }
	}
	l.Pages.Whole(func(bool) {
		post(h.host.Invalidate)
	})
}

// libraryLocateRow returns the displayed row of the playing track: its
// position in the filtered view when a query is active, otherwise in the
// whole library. The header row is row zero.
func (h *Home) libraryLocateRow(l *List, matches []int) (int, bool) {
	cur := h.currentTrack()
	if cur == nil || l == nil || l.Pages == nil {
		return 0, false
	}
	if matches == nil {
		for i := 0; i < l.Pages.Len(); i++ {
			if tr := l.Pages.Peek(i); tr != nil && tr.ID == cur.ID {
				return i + 1, true
			}
		}
		return 0, false
	}
	for row, i := range matches {
		if tr := l.Pages.Peek(i); tr != nil && tr.ID == cur.ID {
			return row + 1, true
		}
	}
	return 0, false
}

// libraryLocatePending fulfils a pending "locate playing" request: it scrolls
// to the playing row and tints it briefly, or asks for the missing pages first.
func (h *Home) libraryLocatePending(c *ui.Context, l *List, matches []int, all bool) {
	if !h.libraryLocate {
		return
	}
	if row, ok := h.libraryLocateRow(l, matches); ok {
		h.trackList.ScrollTo(row, ui.Center)
		if cur := h.currentTrack(); cur != nil {
			h.libraryFlashID = cur.ID
			h.libraryFlashUntil = c.Now().Add(1500 * time.Millisecond)
			c.After(1500 * time.Millisecond)
		}
		h.libraryLocate = false
		return
	}
	if !all {
		h.ensureLibraryWhole(l, h.libraryQuery())
		return
	}
	h.libraryLocate = false
	switch {
	case h.currentTrack() == nil:
		h.host.Toast("还没有正在播放的歌曲")
	case h.libraryQuery() == "":
		h.host.Toast("正在播放的歌曲不在音乐库中")
	default:
		h.host.Toast("当前搜索结果中没有正在播放的歌曲")
	}
}

// librarySearchField is the in-library filter input. Filtering changes only
// what the page shows; playback still starts from the whole library.
func (h *Home) librarySearchField(c *ui.Context) {
	p := paletteFor(c.Theme())
	field := ui.Row(c).Height(32).PaddingX(10).Gap(7).Radius(9).AlignItems(ui.Center).Width(200).Shrink(1).MinWidth(120)
	if c.Focused(h.librarySearchInput) {
		field.Background(p.bg).Shadow(0, 0, 0, 3, p.accent.Alpha(0.25)).Border(1, p.accent.Alpha(0.6))
	} else {
		field.Background(p.hover).Border(1, p.hairline)
	}
	field.Children(func() {
		ui.Icon(c, iconSearchField).Size(14, 14).TextColor(p.muted)
		ui.TextInput(c.Key("fnmusic-library-search"), &h.librarySearch).
			Bind(&h.librarySearchInput).Placeholder("在音乐库中搜索").Label("在音乐库中搜索").FontSize(13).Grow(1).MinWidth(0)
		if h.librarySearch != "" {
			iconButton(c.Key("fnmusic-library-search-clear"), iconX, 11, p.muted, "清空搜索").KeepFocus().OnClick(func() {
				h.librarySearch = ""
				h.libraryWholeOn, h.libraryWholeFor = false, ""
				h.libraryLocate = false
				h.host.Invalidate()
			})
		}
	})
}

// libraryFloatingActions keeps the locator and the back-to-top button beside
// the list as it scrolls, instead of letting them scroll away with the header.
func (h *Home) libraryFloatingActions(c *ui.Context) {
	first, _ := h.trackList.Visible()
	ui.Overlay(c, func() {
		rail := ui.Column(c).Absolute().Right(24).Bottom(playerBarHeight + 16).Gap(8)
		rail.Children(func() {
			if first > 0 {
				if h.libraryFloatingAction(c, "fnmusic-library-top", iconArrowUp, "回到顶部") {
					h.trackList.ScrollTo(0, ui.Start)
				}
			}
			if h.libraryFloatingAction(c, "fnmusic-library-locate", iconLocatePlaying, "定位正在播放") {
				h.libraryLocate = true
				h.host.Invalidate()
			}
		})
	})
}

func (h *Home) libraryFloatingAction(c *ui.Context, key string, icon *ui.SVG, label string) bool {
	p := paletteFor(c.Theme())
	b := ui.ButtonBase(c.Key(key)).Size(40, 40).Radius(999).Center().FocusRing(false).Label(label)
	b.Background(p.sidebar).Border(1, p.hairline).Shadow(0, 8, 20, -8, ui.RGBA(0, 0, 0, 0.3))
	if b.Hovered() {
		b.Background(p.hover)
	}
	b.Children(func() { ui.Icon(c, icon).Size(17, 17).TextColor(p.muted) })
	return b.Clicked()
}

// emptyFilteredTracks shows the header and a way back when a library query
// matches nothing: the search box stays put and a button drops the filter.
func (h *Home) emptyFilteredTracks(c *ui.Context, view TrackView, empty string) {
	p := paletteFor(c.Theme())
	top, bottom := h.insets(c)
	ui.Column(c).Grow(1).Padding(top+pageGap, pagePad-12, bottom+32, pagePad-12).Children(func() {
		ui.Column(c).Children(func() {
			h.startMark(c)
			view.Header(c)
		})
		ui.Column(c).Grow(1).Center().Gap(12).Children(func() {
			ui.Text(c, empty).FontSize(13).TextColor(p.muted)
			if view.OnClearFilter != nil && ui.Button(c, "清空搜索").Clicked() {
				view.OnClearFilter()
			}
		})
	})
}

package fnmusic

import (
	"fmt"
	"slices"

	"github.com/egoist/mygo/ui"
	"github.com/waygu-music-plugin/sdk/pluginapi"
)

func (h *Home) discoverPage(c *ui.Context) {
	groups := groupPlaylists(h.state.Playlists())
	catalog := h.state.Catalog()
	cols, coverWidth := h.cardWidth(c)
	screenWidth, _ := c.Size()
	innerWidth := screenWidth - sidebarWidth - 2*pagePad
	featureCols := 1
	if innerWidth >= 550 {
		featureCols = 2
	}
	featureWidth := (innerWidth - 16*float32(featureCols-1)) / float32(featureCols)
	rows := []func(*ui.Context){func(c *ui.Context) {
		ui.Row(c).AlignItems(ui.Start).Children(func() {
			ui.Box(c).Grow(1).Children(func() { h.heading(c, "在线发现", "") })
			if iconButton(c.Key("discover-refresh"), iconReload, 17, paletteFor(c.Theme()).muted, "刷新在线发现").
				Disabled(catalog.discoveryRefreshing).Clicked() {
				h.refreshDiscovery(catalog)
			}
		})
	}}
	addSection := func(title string) {
		rows = append(rows, func(c *ui.Context) { h.section(c, title).Margin(12, 0, 12, 0) })
	}
	addCards := func(lists []pluginapi.Playlist, across int, width, gap float32, render func(*ui.Context, pluginapi.Playlist)) {
		for chunk := range slices.Chunk(lists, across) {
			rows = append(rows, func(c *ui.Context) {
				ui.Row(c).Gap(gap).Margin(0, 0, 18, 0).AlignItems(ui.Stretch).Children(func() {
					for _, pl := range chunk {
						key := c.Key(pl.ID)
						ui.Box(key).Width(width).MinWidth(0).Children(func() { render(key.Key("card"), pl) })
					}
				})
			})
		}
	}
	if lists := slices.Concat(groups[recommendedPlaylist], groups[accountPlaylist]); len(lists) > 0 {
		addSection("为你推荐")
		addCards(lists, cols, coverWidth, 20, h.discoveryCoverCard)
	}
	if lists := groups[chartPlaylist]; len(lists) > 0 {
		addSection("排行榜")
		featured := min(4, len(lists))
		addCards(lists[:featured], featureCols, featureWidth, 16, h.chartPreviewCard)
		addCards(lists[featured:], cols, coverWidth, 20, h.discoveryCoverCard)
	}
	if lists := groups[publicPlaylist]; len(lists) > 0 {
		addSection("精选歌单")
		addCards(lists, cols, coverWidth, 20, h.discoveryCoverCard)
	}
	if !groups.hasDiscovery() {
		rows = append(rows, func(c *ui.Context) {
			ui.Text(c, "当前账号没有可用的推荐或榜单").FontSize(14).TextColor(paletteFor(c.Theme()).muted)
		})
	}

	top, bottom := h.insets(c)
	h.underTop, h.underBottom = true, true
	ui.List(c.Key("online-discovery"), &h.platformCardList, len(rows), func(i int) {
		ui.Column(c).Children(func() {
			if i == 0 {
				h.startMark(c)
			}
			rows[i](c)
			if i == len(rows)-1 {
				h.endMark(c)
			}
		})
	}).Grow(1).Padding(top+pageGap, pagePad, bottom+48, pagePad).ScrollbarInsets(top, 0, bottom, 0)
}

func (h *Home) refreshDiscovery(catalog *Catalog) {
	catalog.discoveryRefreshing = true
	gen, ctx := h.state.Generation(), h.scope
	go func() {
		_, err := h.state.LoadPlaylists(ctx)
		h.postCurrent(gen, func() {
			catalog.discoveryRefreshing = false
			if err != nil {
				h.host.Toast("刷新在线发现失败")
			} else {
				clear(catalog.discoveryPreviews)
				for id, list := range catalog.Lists {
					if list.Spec.Kind == ListPlaylist && playlistGroup(list.Spec.Ref) != personalPlaylist {
						catalog.Refresh(id)
					}
				}
			}
			h.host.Invalidate()
		})
	}()
}

func (h *Home) openDiscoverPlaylist(id int64) {
	h.navigate(Route{Page: PagePlaylist, ID: id})
}

func (h *Home) playDiscoverPlaylist(id int64) {
	PlayList(h.host.Player(), h.state.Catalog().ListFor(id), 0)
}

func (h *Home) chartPreviewCard(c *ui.Context, pl pluginapi.Playlist) {
	preview := h.state.Catalog().discoveryPreview(pl.Ref)
	preview.At(0)

	p := paletteFor(c.Theme())
	title := playlistDisplayName(pl)
	card := ui.ButtonBase(c).Label(title).FocusRing(false).MinHeight(112).Padding(12).Gap(14).Radius(14).AlignItems(ui.Center)
	card.Background(p.hover)
	if card.Hovered() {
		card.Background(p.press)
	}
	card.Children(func() {
		h.coverAtSize(c, pl.Cover(), 76, 9).Shrink(0)
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(5).Children(func() {
			ui.Text(c, title).FontSize(14).FontWeight(700).SingleLine()
			for i := range discoveryPreviewSize {
				if tr := preview.Peek(i); tr != nil {
					ui.Row(c).Gap(7).Children(func() {
						ui.Textf(c, "%d", i+1).Width(10).FontSize(12).FontWeight(700).TextColor(p.muted)
						ui.RichText(c, ui.Span{Text: tr.Name, Weight: 500},
							ui.Span{Text: "  " + tr.ArtistNames(), Color: p.muted}).
							FontSize(12).SingleLine().Shrink(1).MinWidth(0)
					})
				}
			}
			if preview.Peek(0) == nil && pl.TrackCount > 0 {
				ui.Textf(c, "%d 首", pl.TrackCount).FontSize(12).TextColor(p.muted)
			}
		})
	})
	if card.Clicked() {
		h.openDiscoverPlaylist(pl.ID)
	}
}

func (h *Home) discoveryCoverCard(c *ui.Context, pl pluginapi.Playlist) {
	sub := ""
	if pl.TrackCount > 0 {
		sub = fmt.Sprintf("%d 首", pl.TrackCount)
	}
	if h.coverCard(c, playlistDisplayName(pl), pl.Cover(), 0, sub, func() { h.playDiscoverPlaylist(pl.ID) }) {
		h.openDiscoverPlaylist(pl.ID)
	}
}

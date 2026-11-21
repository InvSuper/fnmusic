package fnmusic

import (
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"
	backend "github.com/waygu-music-plugin/fnmusic/internal/client"
	"github.com/waygu-music-plugin/sdk/pluginapi"
	"github.com/waygu-music-plugin/sdk/pluginapi/paging"
	"github.com/waygu-music-plugin/sdk/plugintest"
)

// Virtual playlist GUIDs determine the section and editability even when ordinary playlists have the same name.
func TestDiscoveryGroupsUsePlaylistRefs(t *testing.T) {
	ordinary := pluginapi.Playlist{ID: 1, Ref: "native-guid", Name: "每日推荐"}
	daily := pluginapi.Playlist{ID: 2, Ref: "online:playlist:daily:20261121:user", Name: ordinary.Name}
	liked := pluginapi.Playlist{ID: 3, Ref: "online:playlist:liked:user"}
	hot := pluginapi.Playlist{ID: 4, Ref: "online:playlist:hot:20261121"}
	chart := pluginapi.Playlist{ID: 5, Ref: "online:playlist:public:musicbox:netease:chart:3778678"}
	public := pluginapi.Playlist{ID: 6, Ref: "online:playlist:public:lx:kw:playlist:kw%3Atest"}
	account := pluginapi.Playlist{ID: 7, Ref: "online:playlist:nm:123"}
	unknown := pluginapi.Playlist{ID: 8, Ref: "online:playlist:public:lx:kw:unknown:123"}
	incomplete := pluginapi.Playlist{ID: 9, Ref: "online:playlist:public:musicbox:netease:chart:"}
	lists := []pluginapi.Playlist{chart, ordinary, daily, public, hot, liked, account, unknown, incomplete}
	groups := groupPlaylists(lists)
	for kind, want := range map[playlistKind][]int64{
		personalPlaylist:    {1, 8, 9},
		recommendedPlaylist: {2, 4, 3},
		chartPlaylist:       {5},
		publicPlaylist:      {6},
		accountPlaylist:     {7},
	} {
		var ids []int64
		for _, pl := range groups[kind] {
			ids = append(ids, pl.ID)
		}
		if !slices.Equal(ids, want) {
			t.Fatalf("group %v: got %v, want %v", kind, ids, want)
		}
	}
	s := NewState()
	s.SetPlaylists(lists)
	if got := s.personalPlaylists(); len(got) != 3 || got[0].Ref != ordinary.Ref {
		t.Fatalf("editable destinations = %+v", got)
	}
}

// Playlist titles drop extension labels but preserve meaningful brackets and native playlist names.
func TestDiscoveryPlaylistDisplayNames(t *testing.T) {
	const daily = "online:playlist:daily:20261121:user"
	const chart = "online:playlist:public:musicbox:netease:chart:3778678"
	const public = "online:playlist:public:lx:kw:playlist:123"
	for _, tt := range []struct{ ref, name, want string }{
		{daily, "每日推荐11-21", "今日精选"},
		{daily, "每日推荐 2026-11-21", "今日精选"},
		{chart, "[网易] 飙升榜", "飙升榜"},
		{public, "【酷我】 [纯音乐] 钢琴", "[纯音乐] 钢琴"},
		{public, "[纯音乐] 夜曲", "[纯音乐] 夜曲"},
		{"native-guid", "[网易] 每日推荐11-21", "[网易] 每日推荐11-21"},
	} {
		if got := playlistDisplayName(pluginapi.Playlist{Ref: tt.ref, Name: tt.name}); got != tt.want {
			t.Errorf("title for %q = %q, want %q", tt.name, got, tt.want)
		}
	}
}

// Chart and recommendation cards open the intended server playlists, and a plain-server login drops the discovery entry.
func TestDiscoveryChartNavigationFollowsAccount(t *testing.T) {
	s := NewState()
	client, err := backend.New(backend.Config{Address: "127.0.0.1:1", Username: "u", PassHash: "p"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Attach(client); err != nil {
		t.Fatal(err)
	}
	native := pluginapi.Playlist{ID: 21, Ref: "ordinary", Name: "自建榜单"}
	chart := pluginapi.Playlist{ID: 22, Ref: "online:playlist:public:musicbox:netease:chart:3778678", Name: "[网易] 飙升榜"}
	daily := pluginapi.Playlist{ID: 23, Ref: "online:playlist:daily:20261121:user", Name: "每日推荐11-21"}
	account := pluginapi.Playlist{ID: 24, Ref: "online:playlist:nm:123", Name: "收藏的专辑"}
	s.SetPlaylists([]pluginapi.Playlist{native, chart, daily, account})
	h := NewHome(s, plugintest.NewHost(ID))
	preview := paging.New[pluginapi.Track](discoveryPreviewSize, 0, nil, nil)
	preview.Land(0, []pluginapi.Track{{Name: "第一名"}}, 1)
	s.Catalog().discoveryPreviews = map[string]*paging.Pager[pluginapi.Track]{chart.Ref: preview}
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Row(c).Fill().Children(func() {
			ui.Column(c).Width(sidebarWidth).Padding(14).Children(func() { h.sidebar(c) })
			ui.Column(c).Grow(1).Children(func() {
				if h.Route().Page == PageDiscover {
					h.Content(c)
				}
			})
		})
	}, 1180, 780)
	if err := tt.Click("在线发现"); err != nil {
		t.Fatal(err)
	}
	if h.Route().Page != PageDiscover {
		t.Fatalf("discovery route = %+v", h.Route())
	}
	for _, ttCase := range []struct {
		playlist pluginapi.Playlist
		title    string
	}{{chart, "飙升榜"}, {daily, "今日精选"}, {account, account.Name}} {
		if err := tt.Click(ttCase.title); err != nil {
			t.Fatal(err)
		}
		if route := h.Route(); route.Page != PagePlaylist || route.ID != ttCase.playlist.ID {
			t.Fatalf("route for %q = %+v", ttCase.title, route)
		}
		list := s.Catalog().ListFor(ttCase.playlist.ID)
		if list == nil || list.Spec.Ref != ttCase.playlist.Ref || list.Name != ttCase.title {
			t.Fatalf("opened %q with the wrong ref or title: %+v", ttCase.title, list)
		}
		h.Back()
		tt.Frame()
		if h.Route().Page != PageDiscover {
			t.Fatalf("after Back: %+v", h.Route())
		}
	}

	s.Detach()
	if err := s.Attach(client); err != nil {
		t.Fatal(err)
	}
	s.SetPlaylists([]pluginapi.Playlist{native})
	h.ResetNavigation(Route{Page: PageLibrary})
	tt.Frame()
	if _, shown := tt.Find("在线发现"); shown {
		t.Fatal("a plain server inherited the previous account's discovery entry")
	}
	if err := tt.Click(native.Name); err != nil {
		t.Fatal(err)
	}
	if route := h.Route(); route.Page != PagePlaylist || route.ID != native.ID {
		t.Fatalf("ordinary playlist route = %+v", route)
	}
}

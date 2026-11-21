package fnmusic

import (
	"net/url"
	"slices"
	"strconv"
	"strings"

	backend "github.com/waygu-music-plugin/fnmusic/internal/client"
	"github.com/waygu-music-plugin/sdk/pluginapi"
	"github.com/waygu-music-plugin/sdk/pluginapi/paging"
)

type playlistKind int

const (
	personalPlaylist playlistKind = iota
	recommendedPlaylist
	chartPlaylist
	publicPlaylist
	accountPlaylist
	playlistKindCount
)

// The extension adds virtual playlists to the standard account response.
// Their GUIDs carry the type even when names or the configured provider change.
func playlistGroup(ref string) playlistKind {
	const prefix = "online:playlist:"
	tail, ok := strings.CutPrefix(ref, prefix)
	if !ok {
		return personalPlaylist
	}
	kind, id, ok := strings.Cut(tail, ":")
	if !ok || id == "" {
		return personalPlaylist
	}
	switch kind {
	case "daily", "liked", "hot":
		return recommendedPlaylist
	case "nm":
		if value, err := strconv.ParseInt(id, 10, 64); err == nil && value > 0 {
			return accountPlaylist
		}
	case "public":
		parts := strings.SplitN(id, ":", 4)
		if len(parts) != 4 || parts[0] == "" || parts[1] == "" {
			break
		}
		if decoded, err := url.PathUnescape(parts[3]); err != nil || decoded == "" {
			break
		}
		switch parts[2] {
		case "chart":
			return chartPlaylist
		case "playlist":
			return publicPlaylist
		}
	}
	return personalPlaylist
}

// playlistDisplayName removes presentation metadata from extension playlists.
func playlistDisplayName(pl pluginapi.Playlist) string {
	group := playlistGroup(pl.Ref)
	if group == personalPlaylist {
		return pl.Name
	}
	if strings.HasPrefix(pl.Ref, "online:playlist:daily:") {
		return "今日精选"
	}
	name := strings.TrimSpace(pl.Name)
	open, close := "[", "]"
	if strings.HasPrefix(name, "【") {
		open, close = "【", "】"
	}
	tail, ok := strings.CutPrefix(name, open)
	if !ok {
		return name
	}
	tag, title, ok := strings.Cut(tail, close)
	if !ok || strings.TrimSpace(title) == "" {
		return name
	}
	tag = strings.TrimSpace(tag)
	switch strings.ToLower(tag) {
	case "网易", "网易云", "网易云音乐", "酷我", "酷狗", "qq", "qq音乐", "咪咕", "洛雪":
		return strings.TrimSpace(title)
	}
	if group == chartPlaylist || group == publicPlaylist {
		source := strings.SplitN(pl.Ref, ":", 7)[4]
		if strings.EqualFold(tag, source) {
			return strings.TrimSpace(title)
		}
	}
	return name
}

type playlistGroups [playlistKindCount][]pluginapi.Playlist

func groupPlaylists(playlists []pluginapi.Playlist) playlistGroups {
	var groups playlistGroups
	for _, pl := range playlists {
		kind := playlistGroup(pl.Ref)
		groups[kind] = append(groups[kind], pl)
	}
	return groups
}

func (groups playlistGroups) hasDiscovery() bool {
	for _, lists := range groups[recommendedPlaylist:] {
		if len(lists) > 0 {
			return true
		}
	}
	return false
}

func (s *State) personalPlaylists() []pluginapi.Playlist {
	return slices.DeleteFunc(s.Playlists(), func(pl pluginapi.Playlist) bool {
		return playlistGroup(pl.Ref) != personalPlaylist
	})
}

const discoveryPreviewSize = 3

func (c *Catalog) discoveryPreview(ref string) *paging.Pager[pluginapi.Track] {
	if p := c.discoveryPreviews[ref]; p != nil {
		return p
	}
	if c.discoveryPreviews == nil {
		c.discoveryPreviews = make(map[string]*paging.Pager[pluginapi.Track])
	}
	p := paging.NewScoped(c.scope, discoveryPreviewSize, 0, c.Post, catalogPage(c, func(client *backend.Client, page int) ([]pluginapi.Track, int, error) {
		tracks, total, err := client.PlaylistPage(ref, page, discoveryPreviewSize)
		return tracks, min(total, discoveryPreviewSize), err
	}))
	c.discoveryPreviews[ref] = p
	return p
}

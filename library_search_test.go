package fnmusic

import (
	"slices"
	"testing"

	"github.com/waygu-music-plugin/sdk/pluginapi"
	"github.com/waygu-music-plugin/sdk/pluginapi/paging"
)

// A library search covers every loaded page and returns positions in the
// whole list, so a filtered row still plays from its place in the library.
func TestLibrarySearchMatchesWholeList(t *testing.T) {
	p := paging.New[pluginapi.Track](2, 0, nil, nil)
	p.Land(0, []pluginapi.Track{
		{ID: 1, Name: "Девочка пой", Artists: []pluginapi.Artist{{Name: "AlbertOG"}}},
		{ID: 2, Name: "He гадай", Album: pluginapi.Album{Name: "WINTER"}},
	}, 4)
	p.Land(1, []pluginapi.Track{
		{ID: 3, Name: "我走后", Artists: []pluginapi.Artist{{Name: "卢卢快闭嘴"}}},
		{ID: 4, Name: "0321", Album: pluginapi.Album{Name: "梦想"}},
	}, 4)
	l := &List{Spec: Spec{Kind: ListLibrary}, Pages: p}

	tests := []struct {
		query string
		want  []int
	}{
		{" winter ", []int{1}},
		{"我走后", []int{2}},
		{"梦想", []int{3}},
		{"不存在", []int{}},
	}
	for _, tc := range tests {
		h := &Home{librarySearch: tc.query}
		got, all := h.libraryMatches(l)
		if !all {
			t.Fatalf("%q reported unloaded pages", tc.query)
		}
		if !slices.Equal(got, tc.want) {
			t.Fatalf("%q matched %v, want %v", tc.query, got, tc.want)
		}
	}

	h := &Home{librarySearch: "  "}
	if got, _ := h.libraryMatches(l); got != nil {
		t.Fatalf("an empty query filtered the list: %v", got)
	}
}

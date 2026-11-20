package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/rpc"
	"os"
	"os/exec"
	"sync/atomic"
	"time"

	"github.com/hashicorp/go-hclog"
	plugin "github.com/hashicorp/go-plugin"
	backend "github.com/waygu-music-plugin/fnmusic/internal/client"
	"github.com/waygu-music-plugin/sdk/pluginapi"
)

var handshake = plugin.HandshakeConfig{ProtocolVersion: 1, MagicCookieKey: "WAYGU_HLS_PROBE", MagicCookieValue: "fixture"}
var adapters = plugin.PluginSet{"hls": Transport{}}

type Transport struct{ plugin.NetRPCUnsupportedPlugin }

func (Transport) Server(b *plugin.MuxBroker) (any, error) { return &Endpoint{broker: b}, nil }
func (Transport) Client(b *plugin.MuxBroker, c *rpc.Client) (any, error) {
	return &Remote{broker: b, rpc: c}, nil
}

type Remote struct {
	broker *plugin.MuxBroker
	rpc    *rpc.Client
}
type Endpoint struct {
	broker *plugin.MuxBroker
	hls    *backend.HLS
}
type Description struct {
	Millis      []int64
	TotalMillis int64
}
type Segment struct {
	ID    uint32
	Index int
}

func (e *Endpoint) Open(addr string, out *Description) error {
	c, err := backend.New(backend.Config{Address: addr, Username: "fixture-user", PassHash: backend.HashPassword("fixture-password")})
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	e.hls, err = c.TranscodeHLS(ctx, &pluginapi.Track{Ref: "fixture-track", Format: "m4a", Source: backend.Source})
	if err != nil {
		return err
	}
	for _, segment := range e.hls.Segments {
		out.Millis = append(out.Millis, segment.Duration.Milliseconds())
	}
	out.TotalMillis = e.hls.Total.Milliseconds()
	return nil
}
func (e *Endpoint) Fetch(in Segment, n *int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	data, err := e.hls.Fetch(ctx, in.Index)
	if err != nil {
		return err
	}
	go func() {
		conn, err := e.broker.Dial(in.ID)
		if err != nil {
			return
		}
		defer conn.Close()
		conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
		_, _ = conn.Write(data)
	}()
	*n = len(data)
	return nil
}
func (e *Endpoint) Release(_ bool, _ *bool) error { e.hls.Release(); return nil }

func (e *Endpoint) Lease(id uint32, _ *bool) error {
	hls := e.hls
	go func() {
		conn, err := e.broker.Accept(id)
		if err == nil {
			_, _ = io.Copy(io.Discard, conn)
			_ = conn.Close()
		}
		hls.Release()
		select {
		case <-hls.Released():
		case <-time.After(3 * time.Second):
		}
		os.Exit(0)
	}()
	return nil
}

func payload(marker byte) []byte { return bytes.Repeat([]byte{255, 251, marker}, 100) }
func require(ok bool, explanation string) {
	if !ok {
		panic(explanation)
	}
}
func must(err error) {
	if err != nil {
		panic(err)
	}
}

func main() {
	if os.Getenv(handshake.MagicCookieKey) != "" {
		plugin.Serve(&plugin.ServeConfig{HandshakeConfig: handshake, Plugins: adapters, Logger: hclog.NewNullLogger()})
		return
	}
	if len(os.Args) == 3 && os.Args[1] == "lease-host" {
		c, r := connect()
		defer c.Kill()
		var desc Description
		must(r.rpc.Call("Plugin.Open", os.Args[2], &desc))
		r.broker.NextId()
		id := r.broker.NextId()
		var ack bool
		must(r.rpc.Call("Plugin.Lease", id, &ack))
		_, err := r.broker.Dial(id)
		must(err)
		os.Exit(0)
	}
	checkRenewal(false)
	checkRenewal(true)
	checkLeaseRelease()
}

func connect() (*plugin.Client, *Remote) {
	path, err := os.Executable()
	must(err)
	c := plugin.NewClient(&plugin.ClientConfig{HandshakeConfig: handshake, Plugins: adapters, Cmd: exec.Command(path), AllowedProtocols: []plugin.Protocol{plugin.ProtocolNetRPC}, AutoMTLS: true, Logger: hclog.NewNullLogger()})
	client, err := c.Client()
	must(err)
	value, err := client.Dispense("hls")
	must(err)
	return c, value.(*Remote)
}

func checkRenewal(rotate bool) {
	var revision, quit, head, tail, rotatedManifest, rotatedTail, unsigned atomic.Int32
	var expired atomic.Bool
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/music/api/v1/user/password-login":
			fmt.Fprint(w, `{"code":0,"data":{"userToken":"fixture-token"}}`)
		case "/music/api/v1/sys/config":
			fmt.Fprint(w, `{"code":0,"data":{}}`)
		case "/music/api/v1/track/transcode":
			session := "fixture"
			if revision.Add(1) > 1 && rotate {
				session = "rotated"
			} else {
				expired.Store(false)
			}
			fmt.Fprintf(w, `{"code":0,"data":{"url":"/music/api/v1/hls/%s/index.m3u8"}}`, session)
		case "/music/api/v1/track/transcode/quit":
			quit.Add(1)
			fmt.Fprint(w, `{"code":0}`)
		case "/music/api/v1/hls/fixture/index.m3u8", "/music/api/v1/hls/fixture/head.mp3", "/music/api/v1/hls/fixture/tail.mp3",
			"/music/api/v1/hls/rotated/index.m3u8", "/music/api/v1/hls/rotated/head.mp3", "/music/api/v1/hls/rotated/tail.mp3":
			if r.Header.Get("authx") == "" || r.Header.Get("Cookie") == "" {
				unsigned.Add(1)
				http.Error(w, "unsigned", 401)
				return
			}
			switch r.URL.Path {
			case "/music/api/v1/hls/rotated/index.m3u8":
				rotatedManifest.Add(1)
				fallthrough
			case "/music/api/v1/hls/fixture/index.m3u8":
				fmt.Fprint(w, "#EXTM3U\n#EXTINF:2.0,\nhead.mp3\n#EXTINF:1.5,\ntail.mp3\n#EXT-X-ENDLIST\n")
			case "/music/api/v1/hls/fixture/head.mp3":
				head.Add(1)
				if rotate && expired.Load() {
					http.Error(w, "Gone", 410)
					return
				}
				fallthrough
			case "/music/api/v1/hls/rotated/head.mp3":
				w.Write(payload(1))
			case "/music/api/v1/hls/fixture/tail.mp3":
				tail.Add(1)
				if expired.Load() {
					http.Error(w, "Gone", 410)
					return
				}
				w.Write(payload(2))
			case "/music/api/v1/hls/rotated/tail.mp3":
				rotatedTail.Add(1)
				w.Write(payload(2))
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer fixture.Close()
	c, r := connect()
	defer c.Kill()
	var desc Description
	must(r.rpc.Call("Plugin.Open", fixture.URL, &desc))
	require(len(desc.Millis) == 2 && desc.Millis[0] == 2000 && desc.Millis[1] == 1500 && desc.TotalMillis == 3500, "invalid timing DTO")
	r.broker.NextId()
	fetch := func(index int, want byte) error {
		id := r.broker.NextId()
		var n int
		if err := r.rpc.Call("Plugin.Fetch", Segment{ID: id, Index: index}, &n); err != nil {
			return err
		}
		conn, err := r.broker.Accept(id)
		must(err)
		defer conn.Close()
		must(conn.SetReadDeadline(time.Now().Add(2 * time.Second)))
		got, err := io.ReadAll(io.LimitReader(conn, 4096))
		must(err)
		require(n == len(got) && bytes.Equal(payload(want), got), "segment data differs")
		return nil
	}
	expired.Store(true)
	must(fetch(1, 2))
	require(head.Load() == 0 && revision.Load() == 2, "expected one renewal without fetching the first segment")
	if rotate {
		require(tail.Load() == 1 && rotatedManifest.Load() == 1 && rotatedTail.Load() == 1, "expected retry through the renewed manifest and segment")
		must(fetch(0, 1))
		require(head.Load() == 0, "subsequent fetch used the old manifest")
		fmt.Println("PASS rotating URL: renewed once, loaded the new manifest and read both segments from it")
	} else {
		require(tail.Load() == 2, "expected one retry of the original segment URL")
		must(fetch(0, 1))
		require(head.Load() == 1, "first segment was unavailable after the non-sequential fetch")
	}
	require(unsigned.Load() == 0, "unsigned playlist or segment fetch")
	var ignored bool
	must(r.rpc.Call("Plugin.Release", true, &ignored))
	must(r.rpc.Call("Plugin.Release", true, &ignored))
	deadline := time.Now().Add(2 * time.Second)
	for quit.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	require(quit.Load() == 1, "idempotent release did not reach server")
	if !rotate {
		fmt.Println("PASS unchanged URL: Feiniu HLS client in go-plugin process, timing, signed segment bytes via MuxBroker, 410 renewal, idempotent release")
	}
}

func checkLeaseRelease() {
	released := make(chan struct{}, 1)
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/music/api/v1/user/password-login":
			fmt.Fprint(w, `{"code":0,"data":{"userToken":"fixture-token"}}`)
		case "/music/api/v1/sys/config":
			fmt.Fprint(w, `{"code":0,"data":{}}`)
		case "/music/api/v1/track/transcode":
			fmt.Fprint(w, `{"code":0,"data":{"url":"/music/api/v1/hls/index.m3u8"}}`)
		case "/music/api/v1/hls/index.m3u8":
			fmt.Fprint(w, "#EXTM3U\n#EXTINF:2.0,\nfirst.mp3\n#EXT-X-ENDLIST\n")
		case "/music/api/v1/track/transcode/quit":
			fmt.Fprint(w, `{"code":0}`)
			released <- struct{}{}
		default:
			http.NotFound(w, r)
		}
	}))
	defer fixture.Close()
	exe, err := os.Executable()
	must(err)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, "lease-host", fixture.URL)
	must(cmd.Run())
	select {
	case <-released:
		fmt.Println("PASS lease loss: HLS child released the server session after abrupt host exit")
	case <-ctx.Done():
		panic("lost host lease did not release the HLS session")
	}
}

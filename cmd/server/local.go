package main

import (
	"bufio"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"os"
	"strings"
)

// localAccess lets requests skip the bearer token when LOCAL_NO_AUTH=true and
// the app can only be reached from this machine. The decision is made on the
// TCP peer address. Host, Origin and forwarding headers are only ever used to
// refuse (DNS rebinding, cross-site requests, reverse proxies), never to grant.
type localAccess struct {
	peers []netip.Prefix
}

var loopbackPeers = []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8"), netip.MustParsePrefix("::1/128")}

// newLocalAccess validates the configuration at startup.
//
//   - Go run directly: LISTEN_ADDR must be a loopback address; only loopback
//     peers are trusted.
//   - Docker: the app has to listen on the container interface, so LISTEN_ADDR
//     is ":8080". It is accepted only when PUBLISHED_HOST (the host address the
//     port is published on, set from the same Compose variable as the port
//     mapping) is loopback. Connections forwarded from the host arrive from the
//     container's default gateway, which is then trusted; other containers on
//     the network are not.
func newLocalAccess(listen, published, routeFile string) (*localAccess, error) {
	host, _, e := net.SplitHostPort(listen)
	if e != nil {
		return nil, fmt.Errorf("LISTEN_ADDR %q: %w", listen, e)
	}
	if isLoopbackName(host) {
		return &localAccess{peers: loopbackPeers}, nil
	}
	if !isLoopbackName(published) {
		return nil, errors.New("LOCAL_NO_AUTH=true requires LISTEN_ADDR on 127.0.0.1/::1, or PUBLISHED_HOST=127.0.0.1 when the container port is published only on the host loopback")
	}
	gws, e := defaultGateways(routeFile)
	if e != nil || len(gws) == 0 {
		return nil, fmt.Errorf("LOCAL_NO_AUTH=true: cannot determine the container gateway from %s", routeFile)
	}
	peers := append([]netip.Prefix{}, loopbackPeers...)
	for _, g := range gws {
		peers = append(peers, netip.PrefixFrom(g, g.BitLen()))
	}
	return &localAccess{peers: peers}, nil
}

func isLoopbackName(h string) bool {
	h = strings.ToLower(strings.Trim(h, "[]"))
	if h == "localhost" {
		return true
	}
	a, e := netip.ParseAddr(h)
	return e == nil && a.Unmap().IsLoopback()
}

// defaultGateways reads IPv4 default routes from /proc/net/route (Linux).
func defaultGateways(path string) ([]netip.Addr, error) {
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	var out []netip.Addr
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fs := strings.Fields(sc.Text())
		if len(fs) < 3 || fs[1] != "00000000" {
			continue
		}
		b, e := hex.DecodeString(fs[2])
		if e != nil || len(b) != 4 {
			continue
		}
		var ip [4]byte
		binary.LittleEndian.PutUint32(ip[:], binary.BigEndian.Uint32(b))
		if a := netip.AddrFrom4(ip); !a.IsUnspecified() {
			out = append(out, a)
		}
	}
	return out, sc.Err()
}

func (l *localAccess) allows(r *http.Request) bool {
	if l == nil {
		return false
	}
	// A proxy in front means the peer is not the user; require the token.
	for _, h := range []string{"Forwarded", "X-Forwarded-For", "X-Forwarded-Host", "X-Real-Ip"} {
		if r.Header.Get(h) != "" {
			return false
		}
	}
	ap, e := netip.ParseAddrPort(r.RemoteAddr)
	if e != nil {
		return false
	}
	ip, trusted := ap.Addr().Unmap(), false
	for _, p := range l.peers {
		if p.Contains(ip) {
			trusted = true
			break
		}
	}
	if !trusted {
		return false
	}
	// DNS rebinding: a foreign site whose name resolves to 127.0.0.1 sends its
	// own Host header.
	host := r.Host
	if h, _, e := net.SplitHostPort(host); e == nil {
		host = h
	}
	if !isLoopbackName(host) {
		return false
	}
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		return true
	}
	// CSRF: writes must come from this page. Cross-site JSON requests need a
	// CORS preflight, which the app never grants.
	if o := r.Header.Get("Origin"); o != "" && o != "http://"+r.Host {
		return false
	}
	if s := r.Header.Get("Sec-Fetch-Site"); s != "" && s != "same-origin" {
		return false
	}
	if r.Method == http.MethodPost || r.Method == http.MethodPatch {
		mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
		return mt == "application/json"
	}
	return true
}

func (l *localAccess) String() string {
	s := []string{}
	for _, p := range l.peers {
		s = append(s, p.String())
	}
	return strings.Join(s, ", ")
}

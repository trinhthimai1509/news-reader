package news

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"golang.org/x/net/html"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type Item struct {
	Title       string `xml:"title"`
	URL         string `xml:"link"`
	Description string `xml:"description"`
	Date        string `xml:"pubDate"`
	Enclosure   struct {
		URL  string `xml:"url,attr"`
		Type string `xml:"type,attr"`
	} `xml:"enclosure"`
	Thumbnail struct {
		URL    string `xml:"url,attr"`
		Width  string `xml:"width,attr"`
		Height string `xml:"height,attr"`
	} `xml:"http://search.yahoo.com/mrss/ thumbnail"`
}

// FeedImage is the image the feed itself attaches to an item (VnExpress
// <enclosure>, BBC <media:thumbnail>), used until the article page is read.
func (it Item) FeedImage(adapter string) *Image {
	base, _ := url.Parse("https://" + map[string]string{"bbc": "www.bbc.co.uk", "vnexpress": "vnexpress.net", "tuoitre": "tuoitre.vn"}[adapter] + "/")
	if strings.HasPrefix(it.Enclosure.Type, "image/") {
		if u, ok := ImageURL(it.Enclosure.URL, base); ok {
			return &Image{Src: u}
		}
	}
	if u, ok := ImageURL(it.Thumbnail.URL, base); ok {
		im := &Image{Src: u, Width: dim(it.Thumbnail.Width), Height: dim(it.Thumbnail.Height)}
		if im.Width == 0 || im.Height == 0 {
			im.Width, im.Height = 0, 0
		}
		return im
	}
	return nil
}

type Feed struct {
	Channel struct {
		Items []Item `xml:"item"`
	} `xml:"channel"`
}

// FeedTitle is the channel title as plain text.
func FeedTitle(b []byte, adapter string) string {
	var f struct {
		Channel struct {
			Title string `xml:"title"`
		} `xml:"channel"`
	}
	if xml.Unmarshal(b, &f) != nil {
		return ""
	}
	return PlainIn(f.Channel.Title, adapter)
}

func ParseFeed(b []byte) ([]Item, error) {
	head := strings.ToLower(string(bytes.TrimSpace(b[:min(len(b), 512)])))
	if strings.HasPrefix(head, "<!doctype html") || strings.HasPrefix(head, "<html") {
		return nil, errors.New("nguồn trả về trang HTML thay vì RSS XML")
	}
	var f Feed
	if err := xml.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("RSS không hợp lệ: %w", err)
	}
	if len(f.Channel.Items) == 0 {
		return nil, errors.New("RSS không có bài hoặc định dạng chưa được hỗ trợ")
	}
	return f.Channel.Items, nil
}
func Allowed(raw, adapter string, feed bool) bool {
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "https" || u.User != nil || (u.Port() != "" && u.Port() != "443") {
		return false
	}
	h := strings.ToLower(u.Hostname())
	if adapter == "vnexpress" {
		return h == "vnexpress.net" && (!feed || strings.HasPrefix(u.Path, "/rss/"))
	}
	if adapter == "tuoitre" {
		// Tuổi Trẻ Online only: no subdomain, no /nld/ or other sites hosted
		// under the domain. Feeds as listed on https://tuoitre.vn/rss.htm.
		if h != "tuoitre.vn" {
			return false
		}
		if feed {
			return ttFeedPath.MatchString(u.Path)
		}
		return strings.HasSuffix(u.Path, ".htm") && !strings.HasPrefix(u.Path, "/nld/")
	}
	if adapter == "bbc" {
		if feed {
			return h == "feeds.bbci.co.uk" && strings.HasPrefix(u.Path, "/news/")
		}
		return h == "www.bbc.com" || h == "www.bbc.co.uk" || h == "bbc.com" || h == "bbc.co.uk"
	}
	return false
}

// trackingParam reports query keys that only carry campaign tracking; they are
// dropped so the same article reached via different feeds maps to one URL.
func trackingParam(k string) bool {
	k = strings.ToLower(k)
	return strings.HasPrefix(k, "utm_") || strings.HasPrefix(k, "at_") || k == "fbclid" || k == "gclid" || k == "ocid"
}

// Canonical normalizes a URL that already passed Allowed: lower-case host,
// no default port, no fragment and no tracking parameters. BBC serves the same
// article on bbc.com and bbc.co.uk, so BBC article hosts collapse to one host.
func Canonical(raw, adapter string, feed bool) string {
	u, e := url.Parse(raw)
	if e != nil {
		return raw
	}
	u.Host = strings.ToLower(u.Hostname())
	if adapter == "bbc" && !feed {
		u.Host = "www.bbc.co.uk"
	}
	u.Fragment, u.RawFragment = "", ""
	q := u.Query()
	for k := range q {
		if trackingParam(k) {
			q.Del(k)
		}
	}
	u.RawQuery = q.Encode()
	return u.String()
}

// vnexpressArticle matches the article URL shape checked on 2026-10-05:
// https://vnexpress.net/<slug>-<id>.html. VnExpress routes on the numeric id
// and answers any other slug with a 301 to the current one, so the id is the
// stable identity when an article is retitled.
var vnexpressArticle = regexp.MustCompile(`^/[a-z0-9]+(?:-[a-z0-9]+)*-([0-9]{6,10})\.html$`)

// tuoitreArticle matches https://tuoitre.vn/<slug>-<id>.htm, checked on
// 2026-10-05: any other slug with the same id answers 301 to the current one.
// /video/ pages use a different, shorter id and are not matched.
var tuoitreArticle = regexp.MustCompile(`^/[a-z0-9]+(?:-[a-z0-9]+)*-([0-9]{15,20})\.htm$`)

// ttFeedPath: https://tuoitre.vn/<section>.rss (official list) and the
// equivalent /rss/<section>.rss.
var ttFeedPath = regexp.MustCompile(`^/(?:rss/)?[a-z0-9-]+\.rss$`)

// ArticleKey returns a stable per-source identity for an article URL that
// already passed Allowed and Canonical, or "" when the URL shape is not one
// that was verified; such URLs are identified by their canonical URL only.
func ArticleKey(raw, adapter string) string {
	pattern, host := map[string]*regexp.Regexp{"vnexpress": vnexpressArticle, "tuoitre": tuoitreArticle}[adapter], map[string]string{"vnexpress": "vnexpress.net", "tuoitre": "tuoitre.vn"}[adapter]
	if pattern == nil {
		return ""
	}
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "https" || u.Host != host || u.RawQuery != "" {
		return ""
	}
	m := pattern.FindStringSubmatch(u.Path)
	if m == nil {
		return ""
	}
	return adapter + ":" + m[1]
}

// blockedPrefixes covers special-purpose ranges that net.IP helpers do not
// (CGNAT, benchmarking, documentation, reserved, and IPv6 prefixes that embed
// an IPv4 address and could be translated to an internal host).
var blockedPrefixes = func() []netip.Prefix {
	out := []netip.Prefix{}
	for _, s := range []string{"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "192.88.99.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4",
		"64:ff9b::/96", "64:ff9b:1::/48", "100::/64", "2001::/32", "2001:2::/48", "2001:db8::/32", "2002::/16", "3fff::/20", "5f00::/16"} {
		out = append(out, netip.MustParsePrefix(s))
	}
	return out
}()

// PublicIP reports whether ip is a routable public unicast address.
func PublicIP(ip net.IP) bool {
	a, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	a = a.Unmap()
	if !a.IsGlobalUnicast() || a.IsPrivate() || a.IsLoopback() || a.IsLinkLocalUnicast() {
		return false
	}
	for _, p := range blockedPrefixes {
		if p.Contains(a) {
			return false
		}
	}
	return true
}
func Client(adapter string, feed bool) *http.Client {
	dialer := &net.Dialer{Timeout: 8 * time.Second}
	tr := &http.Transport{TLSHandshakeTimeout: 8 * time.Second, ResponseHeaderTimeout: 12 * time.Second,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, err
			}
			for _, a := range ips {
				if !PublicIP(a.IP) {
					return nil, errors.New("blocked non-public address")
				}
			}
			for _, a := range ips {
				c, e := dialer.DialContext(ctx, network, net.JoinHostPort(a.IP.String(), port))
				if e == nil {
					return c, nil
				}
			}
			return nil, errors.New("cannot connect to source")
		}}
	return &http.Client{Transport: tr, Timeout: 20 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 3 {
			return errors.New("nguồn chuyển hướng quá nhiều lần")
		}
		if !Allowed(req.URL.String(), adapter, feed) {
			return fmt.Errorf("nguồn chuyển hướng tới %s, ngoài phạm vi được phép", req.URL.Host+req.URL.Path)
		}
		return nil
	}}
}

// ErrThrottled means the source asked us to slow down; callers should stop
// requesting it for this round without counting a failed attempt.
var ErrThrottled = errors.New("nguồn đang giới hạn tần suất truy cập")

func Fetch(ctx context.Context, c *http.Client, raw, adapter string, feed bool, etag, modified string) ([]byte, http.Header, bool, error) {
	if !Allowed(raw, adapter, feed) {
		return nil, nil, false, errors.New("URL ngoài nguồn được hỗ trợ")
	}
	req, e := http.NewRequestWithContext(ctx, "GET", raw, nil)
	if e != nil {
		return nil, nil, false, e
	}
	req.Header.Set("User-Agent", "PersonalNewsReader/0.1 (+personal RSS reader)")
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	if modified != "" {
		req.Header.Set("If-Modified-Since", modified)
	}
	resp, e := c.Do(req)
	if e != nil {
		var ue *url.Error
		if errors.As(e, &ue) {
			e = ue.Err
		}
		return nil, nil, false, e
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotModified {
		return nil, resp.Header, true, nil
	}
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable {
		return nil, nil, false, fmt.Errorf("%w: HTTP %d", ErrThrottled, resp.StatusCode)
	}
	if resp.StatusCode != 200 {
		return nil, nil, false, fmt.Errorf("nguồn trả HTTP %d", resp.StatusCode)
	}
	b, e := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024+1))
	if len(b) > 4*1024*1024 {
		return nil, nil, false, errors.New("response too large")
	}
	return b, resp.Header, false, e
}
func Plain(s string) string {
	n, e := html.Parse(strings.NewReader(s))
	if e != nil {
		return ""
	}
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && (n.Data == "script" || n.Data == "style") {
			return
		}
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
			b.WriteByte(' ')
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return strings.Join(strings.Fields(b.String()), " ")
}
func attr(n *html.Node, k string) string {
	for _, a := range n.Attr {
		if a.Key == k {
			return a.Val
		}
	}
	return ""
}
func hasClass(n *html.Node, c string) bool {
	for _, s := range strings.Fields(attr(n, "class")) {
		if s == c {
			return true
		}
	}
	return false
}
func findNode(n *html.Node, match func(*html.Node) bool) *html.Node {
	if n.Type == html.ElementNode && match(n) {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if f := findNode(c, match); f != nil {
			return f
		}
	}
	return nil
}
func text(n *html.Node) string {
	var b bytes.Buffer
	html.Render(&b, n)
	return Plain(b.String())
}

// interactiveVnExpress are game-type widgets that are the article itself; the
// paragraphs around them are only a teaser, so the article is not "full".
// Reader polls (vote, consultative_question) sit inside normal articles and
// are simply skipped.
var interactiveVnExpress = map[string]bool{"crossword": true, "quiz": true, "minigame": true}

// minFullChars guards against labelling a teaser or a media page as full text.
const minFullChars = 300

// Extract returns only the text paragraphs of ExtractArticle.
func Extract(b []byte, adapter string) ([]string, error) {
	page := map[string]string{"vnexpress": "https://vnexpress.net/", "bbc": "https://www.bbc.co.uk/", "tuoitre": "https://tuoitre.vn/"}[adapter]
	c, e := ExtractArticle(b, adapter, page)
	if e != nil {
		return nil, e
	}
	return c.Paragraphs(), nil
}

// vietnam is Vietnam time (UTC+7, no daylight saving).
var vietnam = time.FixedZone("ICT", 7*3600)

// PublishedIn parses an item date of a given source. Tuổi Trẻ writes
// "10/5/2026 3:42:00 PM" (month/day, U+202F before AM/PM) in Vietnam time; the
// article page's article:published_time confirmed the zone on 2026-10-05.
func PublishedIn(s, adapter string, now time.Time) time.Time {
	if adapter == "tuoitre" {
		v := strings.Join(strings.Fields(strings.ReplaceAll(s, "\u202f", " ")), " ")
		if t, e := time.ParseInLocation("1/2/2006 3:04:05 PM", v, vietnam); e == nil {
			if t.After(now.Add(time.Hour)) {
				return now
			}
			return t
		}
	}
	return Published(s, now)
}

// Published parses RSS dates. Missing or unparseable dates fall back to the
// fetch time; dates more than an hour in the future are clamped so a bad feed
// cannot pin an item to the top of the list.
func Published(s string, now time.Time) time.Time {
	s = strings.TrimSpace(s)
	for _, f := range []string{time.RFC1123Z, time.RFC1123, "Mon, 2 Jan 2006 15:04:05 -0700", "Mon, 2 Jan 2006 15:04:05 MST", time.RFC822Z, time.RFC822, time.RFC3339} {
		if t, e := time.Parse(f, s); e == nil {
			if t.After(now.Add(time.Hour)) {
				return now
			}
			return t
		}
	}
	return now
}

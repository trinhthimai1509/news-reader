package news

import (
	"net"
	"strings"
	"testing"
	"time"
)

func TestRejectUnsafeSources(t *testing.T) {
	for _, u := range []string{"http://vnexpress.net/rss/a", "https://vnexpress.net.evil.com/rss/a", "https://user@vnexpress.net/rss/a", "https://127.0.0.1/rss/a", "https://vnexpress.net:8443/rss/a"} {
		if Allowed(u, "vnexpress", true) {
			t.Fatal(u)
		}
	}
	if !Allowed("https://vnexpress.net/rss/tin-moi-nhat.rss", "vnexpress", true) {
		t.Fatal("valid feed rejected")
	}
}
func TestFeedAndUntrustedHTML(t *testing.T) {
	items, e := ParseFeed([]byte(`<rss><channel><item><title>A &amp; B</title><link>https://vnexpress.net/a</link><description><![CDATA[<script>steal()</script><p>Hello &amp; world</p>]]></description></item></channel></rss>`))
	if e != nil || len(items) != 1 {
		t.Fatal(e)
	}
	if Plain(items[0].Description) != "Hello & world" {
		t.Fatal(Plain(items[0].Description))
	}
}
func TestExtractOnlyArticle(t *testing.T) {
	b := []byte(`<p>outside</p><article class="fck_detail"><p>` + long + `</p><script>bad</script><p>` + long + `</p></article><p>outside</p>`)
	p, e := Extract(b, "vnexpress")
	if e != nil || len(p) != 2 || p[0] != long || p[1] != long {
		t.Fatal(p, e)
	}
}
func TestMissingArticle(t *testing.T) {
	if _, e := Extract([]byte(`<html><p>login required</p></html>`), "bbc"); e == nil {
		t.Fatal("must not label missing content as full")
	}
}

func TestCanonicalDropsTracking(t *testing.T) {
	a := Canonical("https://www.bbc.co.uk/news/articles/c1?at_medium=RSS&at_campaign=rss#top", "bbc", false)
	b := Canonical("https://WWW.BBC.COM/news/articles/c1?utm_source=x", "bbc", false)
	if a != "https://www.bbc.co.uk/news/articles/c1" || a != b {
		t.Fatal(a, b)
	}
	if v := Canonical("https://vnexpress.net/a-1.html?utm_source=rss&id=2", "vnexpress", false); v != "https://vnexpress.net/a-1.html?id=2" {
		t.Fatal(v)
	}
	if v := Canonical("https://Feeds.bbci.co.uk/news/rss.xml", "bbc", true); v != "https://feeds.bbci.co.uk/news/rss.xml" {
		t.Fatal(v)
	}
}
func TestPublicIP(t *testing.T) {
	for _, s := range []string{"127.0.0.1", "10.1.2.3", "172.16.0.1", "192.168.1.1", "169.254.169.254", "100.64.0.1", "0.0.0.0", "240.0.0.1", "198.18.0.1",
		"::1", "fc00::1", "fe80::1", "::ffff:127.0.0.1", "::ffff:10.0.0.1", "64:ff9b::a00:1", "2002:a00:1::1", "2001:db8::1", "::"} {
		if PublicIP(net.ParseIP(s)) {
			t.Error("must block", s)
		}
	}
	for _, s := range []string{"151.101.0.81", "2a04:4e42::81"} {
		if !PublicIP(net.ParseIP(s)) {
			t.Error("must allow", s)
		}
	}
}
func TestRejectUnsafeBBCAndAdapter(t *testing.T) {
	for _, c := range [][3]string{{"https://feeds.bbci.co.uk/sport/rss.xml", "bbc", "feed"}, {"https://feeds.bbci.co.uk.evil.net/news/rss.xml", "bbc", "feed"},
		{"https://feeds.bbci.co.uk/news/rss.xml", "vnexpress", "feed"}, {"https://feeds.bbci.co.uk/news/rss.xml", "other", "feed"}, {"https://evil.com/news/x", "bbc", "article"}} {
		if Allowed(c[0], c[1], c[2] == "feed") {
			t.Error(c)
		}
	}
}

var long = strings.TrimSpace(strings.Repeat("Nội dung bài viết đủ dài để được coi là toàn văn. ", 4))

func TestExtractVnExpressSkipsCaptionsAndRelated(t *testing.T) {
	b := []byte(`<article class="fck_detail"><p class="description">Lead</p><p class="Normal">` + long + `</p>
<figure class="tplCaption"><figcaption><p class="Image">Caption. Ảnh: AFP</p></figcaption></figure>
<table class="tplCaption"><tr><td><p class="Image">Table caption</p></td></tr></table>
<p class="Normal"><strong><a href="/x">&gt;&gt; Related story</a></strong></p>
<p class="Normal">` + long + `</p><div data-component="true" data-component-type="tin_xemthem"><p>Xem thêm</p></div></article>`)
	p, e := Extract(b, "vnexpress")
	if e != nil || len(p) != 3 || p[0] != "Lead" {
		t.Fatal(p, e)
	}
	for _, s := range p {
		if strings.Contains(s, "Caption") || strings.Contains(s, "Related") || strings.Contains(s, "Xem thêm") {
			t.Fatal("non-body text extracted:", s)
		}
	}
}
func TestExtractVnExpressInteractiveIsNotFull(t *testing.T) {
	b := []byte(`<article class="fck_detail"><p class="Normal">` + long + `</p><p class="Normal">` + long + `</p><div class="component crossword" data-component="true" data-component-type="crossword"></div></article>`)
	if _, e := Extract(b, "vnexpress"); e == nil {
		t.Fatal("crossword page must not be marked full")
	}
}
func TestExtractBBCOnlyTextBlocks(t *testing.T) {
	b := []byte(`<article><div data-block="headline"><h1>T</h1></div><div data-block="image"><figure><p>Image caption</p></figure></div>
<div data-block="text"><div data-testid="rich-text"><p>` + long + `</p><p>` + long + `</p><p><a href="/newsletters/z1">Sign up for our newsletter</a></p></div></div>
<div data-block="subheadline"><h2>Sub heading</h2></div><div data-block="links"><ul><li><a href="/news/x"><p>Related link</p></a></li></ul></div>
<div data-block="text"><p>Last paragraph.</p></div><div data-block="topicList"><p>Topic</p></div></article>`)
	p, e := Extract(b, "bbc")
	if e != nil || len(p) != 4 || p[2] != "Sub heading" || p[3] != "Last paragraph." {
		t.Fatal(p, e)
	}
}
func TestExtractBBCVideoPageIsNotFull(t *testing.T) {
	b := []byte(`<article><div data-testid="reveal-text-wrapper"><p>Short video description.</p><p>More on this story.</p></div></article>`)
	if _, e := Extract(b, "bbc"); e == nil {
		t.Fatal("video page must keep the summary")
	}
}
func TestPublishedDates(t *testing.T) {
	now := time.Date(2026, 10, 5, 5, 0, 0, 0, time.UTC)
	cases := map[string]time.Time{
		"Mon, 05 Oct 2026 11:32:56 +0700": time.Date(2026, 10, 5, 4, 32, 56, 0, time.UTC),
		"Mon, 05 Oct 2026 04:58:56 GMT":   time.Date(2026, 10, 5, 4, 58, 56, 0, time.UTC),
		"Mon, 5 Oct 2026 04:58:56 GMT":    time.Date(2026, 10, 5, 4, 58, 56, 0, time.UTC),
		"":                                now,
		"tomorrow":                        now,
		"Mon, 05 Oct 2026 09:00:00 GMT":   now, // future beyond 1h is clamped
	}
	for in, want := range cases {
		if got := Published(in, now); !got.Equal(want) {
			t.Errorf("%q: got %v want %v", in, got, want)
		}
	}
}
func TestFeedHTMLErrorIsExplicit(t *testing.T) {
	_, e := ParseFeed([]byte("<!DOCTYPE html><html><body>blocked</body></html>"))
	if e == nil || !strings.Contains(e.Error(), "HTML") {
		t.Fatal(e)
	}
}
func TestExtractVnExpressReaderPollIsNotInteractive(t *testing.T) {
	b := []byte(`<article class="fck_detail"><p class="Normal">` + long + `</p><div data-component="true" data-component-type="vote"><p>Bạn có đồng ý?</p></div><p class="Normal">` + long + `</p></article>`)
	p, e := Extract(b, "vnexpress")
	if e != nil || len(p) != 2 {
		t.Fatal("article with a reader poll must still be full:", p, e)
	}
}

func TestArticleKeyVnExpress(t *testing.T) {
	// Real pair seen on 2026-10-05: the old slug answers 301 to the new one.
	a := ArticleKey(Canonical("https://vnexpress.net/thu-tuong-nhat-goi-trung-quoc-la-lang-gieng-quan-trong-5128557.html", "vnexpress", false), "vnexpress")
	b := ArticleKey(Canonical("https://vnexpress.net/thu-tuong-nhat-diu-giong-voi-trung-quoc-giua-cang-thang-5128557.html?utm_source=rss", "vnexpress", false), "vnexpress")
	if a != "vnexpress:5128557" || a != b {
		t.Fatalf("keys %q %q", a, b)
	}
	if k := ArticleKey("https://vnexpress.net/a-5128310.html", "vnexpress"); k != "vnexpress:5128310" {
		t.Fatal(k)
	}
}

func TestArticleKeyUnrecognisedFallsBack(t *testing.T) {
	for _, u := range []string{
		"https://vnexpress.net/khai-mac-ngay-hoi-2026-5128002-tong-thuat.html", // live coverage page
		"https://vnexpress.net/giai-tri/phim/thu-vien-phim/heart-of-the-beast-923",
		"https://vnexpress.net/the-thao/a-5128310.html", // nested path: not verified
		"https://vnexpress.net/a-5128310.html?id=2",     // non-tracking query kept by Canonical
		"https://vnexpress.net/a-123.html",              // too short to be an article id
		"https://e.vnexpress.net/news/a-5128310.html",
		"http://vnexpress.net/a-5128310.html",
		"https://vnexpress.net/A-5128310.html",
	} {
		if k := ArticleKey(u, "vnexpress"); k != "" {
			t.Errorf("%s: %q", u, k)
		}
	}
	// BBC keeps URL identity; its tracking cleanup is unchanged.
	if k := ArticleKey(Canonical("https://www.bbc.com/news/articles/c1-5128310.html?at_medium=RSS", "bbc", false), "bbc"); k != "" {
		t.Fatal(k)
	}
}

package news

import (
	"net/url"
	"strings"
	"testing"
)

const vnePage = "https://vnexpress.net/bai-viet-123.html"
const bbcPage = "https://www.bbc.co.uk/news/articles/c1"

func vneBody(inner string) []byte {
	return []byte(`<html><head><meta name="author" content="VnExpress"><meta property="og:image" content="https://i1-vnexpress.vnecdn.net/og.jpg?w=1200&s=x"><meta property="og:image:alt" content="Tiêu đề bài"></head><body>
<article class="fck_detail "><p class="description">` + long + `</p><p class="Normal">` + long + `</p>` + inner + `</article></body></html>`)
}

func TestVnExpressAuthors(t *testing.T) {
	cases := map[string][]string{
		`<p class="Normal" style="text-align:right;"><strong>Phương Thảo</strong> (theo <em>Robb Report</em>)</p>`: {"Phương Thảo"},
		`<p align="right"><b>Hoàng Thông</b> (theo <i>France Football</i>)</p>`:                                    {"Hoàng Thông"},
		`<p class="Normal" style="text-align:right;"><strong>Hoàng Phương - Ngọc Thành</strong></p>`:               {"Hoàng Phương", "Ngọc Thành"},
		`<p class="Normal" style="text-align:right;">Nội dung: Hoài Phương - Thiết kế: Thái Hưng</p>`:              {"Hoài Phương"},
		`<p class="Normal" style="text-align:right;"><strong>Việt Thành tổng hợp</strong></p>`:                     {"Việt Thành"},
		`<p class="Normal" style="text-align:right;"><strong>Nguyễn Tiến</strong> (Ảnh: Google Earth)</p>`:         {"Nguyễn Tiến"},
		`<p class="Normal" style="text-align:right;">Theo Reuters</p>`:                                             nil, // attribution, not an author
		`<p class="Normal">Bài không có dòng ký tên.</p>`:                                                          nil,
		// A right-aligned quote in the middle is not the sign-off.
		`<p class="Normal" style="text-align:right;"><strong>Ông Nguyễn Văn A</strong></p><p class="Normal">` + long + `</p>`:                                                                nil,
		`<p class="Normal" style="text-align:right;"><strong>Hà Thu</strong></p><div data-component="true" data-component-type="tin_xemthem"><p style="text-align:right">Tin khác</p></div>`: {"Hà Thu"},
	}
	for in, want := range cases {
		c, e := ExtractArticle(vneBody(in), "vnexpress", vnePage)
		if e != nil || strings.Join(c.Authors, "|") != strings.Join(want, "|") {
			t.Errorf("%s\n got %q (%v) want %q", in, c.Authors, e, want)
		}
		for _, p := range c.Paragraphs() {
			for _, a := range want {
				if strings.Contains(p, a) && len(want) > 0 {
					t.Errorf("sign-off left in body: %q", p)
				}
			}
		}
	}
}

func bbcBody(inner string) []byte {
	return []byte(`<html><head><meta property="og:image" content="https://ichef.bbci.co.uk/ace/branded_news/1200/cpsprodpb/a/live/hero-1.jpg"></head><body><article>` + inner +
		`<div data-block="text"><p>` + long + `</p><p>` + long + `</p></div></article></body></html>`)
}

func TestBBCAuthors(t *testing.T) {
	single := `<div data-block="byline"><div data-testid="single-byline"><div><span>By</span><span><div><a href="/topics/x"><span>Emma Simpson</span><span><svg><path d="M1"></path></svg></span></a></div></span><div><span>Business correspondent</span></div><div><span><picture><img alt="Emma Simpson" src="https://ichef.bbci.co.uk/ace/standard/160/avatar.png" width="800" height="800"></picture></span></div></div></div></div>`
	multi := `<div data-block="byline"><div data-testid="multi-byline"><span>By</span><span><span>Anna Collinson</span></span><span><span> and </span><span>Jo Adnitt</span><span><span aria-hidden="true">, </span>BBC News Investigations</span></span></div></div>`
	for in, want := range map[string]string{single: "Emma Simpson", multi: "Anna Collinson|Jo Adnitt", "": ""} {
		c, e := ExtractArticle(bbcBody(in), "bbc", bbcPage)
		if e != nil || strings.Join(c.Authors, "|") != want {
			t.Errorf("got %q (%v) want %q", c.Authors, e, want)
		}
		if c.Images() != 0 {
			t.Error("byline avatar must not become an article image")
		}
	}
}

func TestVnExpressImages(t *testing.T) {
	in := `<figure class="tplCaption"><meta itemprop="width" content="1020"><meta itemprop="height" content="636.65"><picture><source data-srcset="https://i1-vnexpress.vnecdn.net/a.jpg?w=1020 1x, https://i1-vnexpress.vnecdn.net/a.jpg?w=2040 2x, javascript:alert(1) 3x">
<img alt="Ảnh A" class="lazy" src="data:image/gif;base64,R0lGOD" data-src="https://i1-vnexpress.vnecdn.net/a.jpg?w=1020"></picture>
<figcaption><p class="Image">Chú thích ảnh A. Ảnh: <em>AFP</em></p></figcaption></figure>
<p class="Normal">Đoạn giữa.</p>
<table class="tplCaption"><tr><td><img src="//i1-vnexpress.vnecdn.net/b.jpg"></td></tr><tr><td><p class="Image">Chú thích B</p></td></tr></table>
<figure><img src="http://i1-vnexpress.vnecdn.net/insecure.jpg"></figure>
<figure><img src="https://evil.example/tracker.png"></figure>
<div data-component="true" data-component-type="tin_xemthem"><figure><img src="https://i1-vnexpress.vnecdn.net/related.jpg"></figure></div>
<div class="item_slide_show"><div class="block_thumb_slide_show" data-src="https://i1-vnexpress.vnecdn.net/s1.jpg?w=1200"><picture><img src="https://i1-vnexpress.vnecdn.net/s1.jpg?w=220" data-src="https://i1-vnexpress.vnecdn.net/s1.jpg?w=1200"></picture>
<div class="desc_cation"><p class="Normal">Lời ảnh slideshow.</p></div></div><div class="desc_cation"><p class="Normal">Lời ảnh slideshow.</p></div></div>`
	c, e := ExtractArticle(vneBody(in), "vnexpress", vnePage)
	if e != nil {
		t.Fatal(e)
	}
	order := ""
	for _, b := range c.Blocks {
		order += b.Type[:1]
	}
	if order != "ppipiip" {
		t.Fatalf("order %q", order)
	}
	a, b, s := c.Blocks[2].Image, c.Blocks[4].Image, c.Blocks[5].Image
	if a.Src != "https://i1-vnexpress.vnecdn.net/a.jpg?w=1020" || a.Caption != "Chú thích ảnh A." || a.Credit != "AFP" || a.Alt != "Ảnh A" || a.Width != 1020 || a.Height != 637 {
		t.Errorf("lazy figure: %+v", a)
	}
	if strings.Contains(a.Srcset, "javascript") || strings.Count(a.Srcset, "vnecdn") != 2 {
		t.Errorf("srcset not cleaned: %q", a.Srcset)
	}
	if b.Src != "https://i1-vnexpress.vnecdn.net/b.jpg" || b.Caption != "Chú thích B" || b.Alt != "" {
		t.Errorf("protocol-relative table image: %+v", b)
	}
	if s.Src != "https://i1-vnexpress.vnecdn.net/s1.jpg?w=1200" || s.Caption != "" {
		t.Errorf("slideshow image: %+v", s)
	}
	if n := strings.Count(strings.Join(c.Paragraphs(), "\n"), "Lời ảnh slideshow."); n != 1 {
		t.Errorf("slideshow narration repeated %d times", n)
	}
	for _, p := range c.Paragraphs() {
		if strings.Contains(p, "Chú thích") {
			t.Error("caption leaked into paragraphs")
		}
	}
	if c.Lead != nil {
		t.Error("VnExpress og:image must not repeat when the body has images")
	}
	c, _ = ExtractArticle(vneBody(""), "vnexpress", vnePage)
	if c.Lead == nil || c.Lead.Src != "https://i1-vnexpress.vnecdn.net/og.jpg?w=1200&s=x" || c.Lead.Alt != "Tiêu đề bài" {
		t.Errorf("lead without body images: %+v", c.Lead)
	}
}

func TestBBCImages(t *testing.T) {
	img := func(file, w, h, cap, credit, alt string) string {
		s := `<div data-block="image"><div data-testid="image"><figure><div><span><picture><source srcSet="https://ichef.bbci.co.uk/ace/standard/240/x/` + file + `.webp 240w" type="image/webp"><img alt="` + alt + `" loading="lazy" src="https://ichef.bbci.co.uk/ace/standard/976/x/` + file + `" srcSet="https://ichef.bbci.co.uk/ace/standard/240/x/` + file + ` 240w, https://ichef.bbci.co.uk/ace/standard/976/x/` + file + ` 976w" width="` + w + `" height="` + h + `"></picture></span>`
		if credit != "" {
			s += `<span role="text"><span>Image source, </span>` + credit + `</span>`
		}
		s += `</div>`
		if cap != "" {
			s += `<figcaption><span>Image caption, </span><div><p>` + cap + `</p></div></figcaption>`
		}
		return s + `</figure></div></div></div>`
	}
	in := img("hero-1.jpg", "2560", "1440", "", "Getty Images", "A street") +
		`<div data-block="text"><p>` + long + `</p></div>` +
		img("b.jpg", "1600", "900", "A caption", "", "Alt b") +
		`<div data-block="links"><ul><li><a href="/news/x"><img src="https://ichef.bbci.co.uk/ace/standard/240/related.jpg"><p>Related</p></a></li></ul></div>` +
		img("banner.png", "1920", "316", "", "", "News Daily banner") +
		img("wide.png", "1280", "100", "", "", "") +
		`<div data-block="media"><img src="https://ichef.bbci.co.uk/ace/standard/976/video-poster.jpg"></div>`
	c, e := ExtractArticle(bbcBody(in), "bbc", bbcPage)
	if e != nil {
		t.Fatal(e)
	}
	if c.Images() != 2 {
		t.Fatalf("images: %d %+v", c.Images(), c.Blocks)
	}
	h, b := c.Blocks[0].Image, c.Blocks[2].Image
	if h.Credit != "Getty Images" || h.Caption != "" || h.Width != 2560 || !strings.Contains(h.Srcset, "976w") || strings.Contains(h.Srcset, "webp") {
		t.Errorf("hero: %+v", h)
	}
	if b.Caption != "A caption" || b.Credit != "" || b.Alt != "Alt b" {
		t.Errorf("captioned: %+v", b)
	}
	if c.Lead != nil {
		t.Error("og:image equal to the hero image must not repeat")
	}
	// og:image not in the body (BBC renders some hero images with JavaScript):
	// kept, switched from the branded rendition to the standard one.
	c, _ = ExtractArticle(bbcBody(img("other.jpg", "1600", "900", "C", "", "")), "bbc", bbcPage)
	if c.Lead == nil || c.Lead.Src != "https://ichef.bbci.co.uk/ace/standard/1200/cpsprodpb/a/live/hero-1.jpg" {
		t.Errorf("lead: %+v", c.Lead)
	}
}

func TestImageURLValidation(t *testing.T) {
	base := mustURL(bbcPage)
	for raw, want := range map[string]string{
		"https://ichef.bbci.co.uk/a.jpg":          "https://ichef.bbci.co.uk/a.jpg",
		"//i1-vnexpress.vnecdn.net/a.jpg":         "https://i1-vnexpress.vnecdn.net/a.jpg",
		"https://I1-THETHAO.vnecdn.net/a.png#x":   "https://i1-thethao.vnecdn.net/a.png",
		"/images/a.jpg":                           "", // relative to www.bbc.co.uk, not an image CDN
		"http://ichef.bbci.co.uk/a.jpg":           "",
		"https://ichef.bbci.co.uk.evil.com/a.jpg": "",
		"https://user@ichef.bbci.co.uk/a.jpg":     "",
		"https://ichef.bbci.co.uk:8443/a.jpg":     "",
		"https://vnecdn.net.evil.com/a.jpg":       "",
		"data:image/png;base64,AAAA":              "",
		"javascript:alert(1)":                     "",
	} {
		got, ok := ImageURL(raw, base)
		if (want == "") == ok || got != want {
			t.Errorf("%q: got %q %v", raw, got, ok)
		}
	}
}

func TestFeedImage(t *testing.T) {
	items, _ := ParseFeed([]byte(`<rss xmlns:media="http://search.yahoo.com/mrss/"><channel>
<item><title>A</title><link>https://vnexpress.net/a.html</link><enclosure type="image/jpeg" url="https://i1-vnexpress.vnecdn.net/a.jpg?w=1200"/></item>
<item><title>B</title><link>https://www.bbc.co.uk/news/b</link><media:thumbnail width="240" height="135" url="https://ichef.bbci.co.uk/ace/standard/240/b.jpg"/></item>
<item><title>C</title><link>https://www.bbc.co.uk/news/c</link><media:thumbnail url="https://evil.example/c.jpg"/></item></channel></rss>`))
	if im := items[0].FeedImage("vnexpress"); im == nil || im.Src != "https://i1-vnexpress.vnecdn.net/a.jpg?w=1200" {
		t.Errorf("enclosure: %+v", im)
	}
	if im := items[1].FeedImage("bbc"); im == nil || im.Width != 240 || im.Height != 135 {
		t.Errorf("thumbnail: %+v", im)
	}
	if im := items[2].FeedImage("bbc"); im != nil {
		t.Errorf("foreign host accepted: %+v", im)
	}
}

func mustURL(s string) *url.URL {
	u, e := url.Parse(s)
	if e != nil {
		panic(e)
	}
	return u
}

// Real VnExpress photo pages contain slideshow items without the thumbnail
// wrapper; this crashed the worker (nil pointer) before.
func TestVnExpressSlideshowVariants(t *testing.T) {
	in := `<div class="item_slide_show"><div class="desc_cation"><p class="Normal">Lời ảnh không có khung.</p></div></div>
<div class="item_slide_show"><img data-src="https://i1-vnexpress.vnecdn.net/s2.jpg"><div class="desc_cation"><p class="Normal">Lời ảnh hai.</p></div></div>
<div class="item_slide_show"></div>`
	c, e := ExtractArticle(vneBody(in), "vnexpress", vnePage)
	if e != nil || c.Images() != 1 || !strings.Contains(strings.Join(c.Paragraphs(), "|"), "Lời ảnh không có khung.") {
		t.Fatalf("%v %d %q", e, c.Images(), c.Paragraphs())
	}
}
func TestExtractNeverPanics(t *testing.T) {
	for _, in := range []string{``, `<article class="fck_detail"><figure></figure><table class="tplCaption"></table><div class="item_slide_show"></div></article>`,
		`<article><div data-block="image"></div><div data-block="byline"><div data-testid="single-byline"></div></div><div data-block="byline"><div data-testid="multi-byline"><span>By</span><span></span></div></div></article>`} {
		for _, ad := range []string{"vnexpress", "bbc"} {
			ExtractArticle([]byte(in), ad, vnePage) // must return, not panic
		}
	}
}

func TestVnExpressVideoEmbedIsNotAnImage(t *testing.T) {
	in := `<figure class="item_slide_show clearfix"><div class="box_embed_video_parent" data-vid="1"><div class="box_img_video"><img class="thumb-above-video" src="https://iv1.vnecdn.net/poster.jpg" alt="Video"></div></div><figcaption><p class="Image">Bài hát. Video: Bilibili</p></figcaption></figure>
<figure class="tplCaption"><img src="https://i1-giaitri.vnecdn.net/photo.jpg"><figcaption><p class="Image">Ảnh thật. Ảnh: Ifeng</p></figcaption></figure>`
	c, e := ExtractArticle(vneBody(in), "vnexpress", vnePage)
	// The poster belongs to the video block, never to the article photos.
	if e != nil || c.Images() != 1 || c.Blocks[3].Image.Src != "https://i1-giaitri.vnecdn.net/photo.jpg" {
		t.Fatalf("%v %+v", e, c.Blocks)
	}
	if v := c.Blocks[2].Video; v == nil || v.Kind != "link" || v.Poster != "https://iv1.vnecdn.net/poster.jpg" || v.Caption != "Bài hát." || v.Credit != "Bilibili" {
		t.Fatalf("video %+v", c.Blocks[2])
	}
	if strings.Contains(strings.Join(c.Paragraphs(), "|"), "Video:") {
		t.Fatal("video caption leaked into text")
	}
}

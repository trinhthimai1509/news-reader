package news

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestVideoSrcValidation(t *testing.T) {
	ok := map[string]string{
		"https://cdn2.tuoitre.vn/471584752817336320/2026/10/5/x.mp4": "mp4",
	}
	for u, k := range ok {
		if got, valid := VideoSrc(k, u); !valid || got != u {
			t.Errorf("%s rejected", u)
		}
	}
	bad := [][2]string{
		// VnExpress playlists are hotlink-protected: never a playable kind.
		{"hls", "https://d1.vnecdn.net/vnexpress/video/video/web/mp4/,240p,360p,480p,,/2026/10/02/x/vne/master.m3u8"},
		{"hls", "http://d1.vnecdn.net/vnexpress/video/video/web/mp4/a.m3u8"},
		{"hls", "https://d1.vnecdn.net/other/a.m3u8"},
		{"hls", "https://d1.vnecdn.net/vnexpress/video/a.m3u8"}, // not the verified layout
		{"hls", "https://d1.vnecdn.net/giaitri/video/video/web/mp4/a.mp4"},
		{"hls", "https://evil.example/vnexpress/video/a.m3u8"},
		{"hls", "https://d1.vnecdn.net/vnexpress/video/video/web/mp4/a.m3u8?token=x"},
		{"hls", "https://user@d1.vnecdn.net/vnexpress/video/video/web/mp4/a.m3u8"},
		{"mp4", "https://cdn2.tuoitre.vn/1.1/?vid=a.mp4"},
		{"mp4", "https://cdn2.tuoitre.vn:8443/a.mp4"},
		{"mp4", "https://d1.vnecdn.net/vnexpress/video/a.mp4"}, // right host for another kind
		{"embed", "https://www.bbc.com/ws/av-embeds/articles/x/p0/en-GB"},
		{"mp4", "javascript:alert(1)"},
	}
	for _, b := range bad {
		if _, valid := VideoSrc(b[0], b[1]); valid {
			t.Errorf("%v accepted", b)
		}
	}
}

const vneVideoFig = `<figure class="item_slide_show clearfix"><div id="video_parent_455512" class="box_embed_video_parent" data-vid="455512" data-duration="113" data-vwidth="1920" data-vheight="1080">
<div data-vid="455512" class="box_img_video embed-container"><img class="thumb-above-video" src="https://iv1.vnecdn.net/vnexpress/images/web/2026/10/02/poster.jpg?w=0&h=0&q=100&dpr=1&fit=crop&s=x" alt="Tiêu đề video"></div>
<div id="embed_video_455512" class="box_embed_video" style="display:none"><div id="videoContainter_455512"><video id="media-video-455512" preload="auto" src="https://d1.vnecdn.net/vnexpress/video/video/web/mp4/,240p,360p,480p,,/2026/10/02/x/vne/master.m3u8" type="application/x-mpegURL" controls></video></div>
<div class="parser_title" style="display:none;">Tiêu đề video</div></div></div>
<figcaption class="desc_cation"><div class="inner_caption"><p class="Image">Cảnh hỗn loạn trên máy bay. Video: <em>Channel 12</em></p></div></figcaption></figure>`

func TestVnExpressVideoBlock(t *testing.T) {
	c, e := ExtractArticle(vneBody(vneVideoFig+`<p class="Normal">`+long+`</p>`), "vnexpress", vnePage)
	if e != nil || !c.HasVideo() || c.Images() != 0 {
		t.Fatalf("%v %+v", e, c.Blocks)
	}
	if c.Blocks[2].Type != "video" || c.Blocks[3].Type != "p" {
		t.Fatalf("order %+v", c.Blocks)
	}
	v := c.Blocks[2].Video
	// Link-only: poster, caption and page link; the playlist URL is not kept.
	if v.Kind != "link" || v.Src != "" || !strings.HasPrefix(v.Poster, "https://iv1.vnecdn.net/") || v.Width != 1920 || v.Height != 1080 ||
		v.Duration != 113 || v.Caption != "Cảnh hỗn loạn trên máy bay." || v.Credit != "Channel 12" || v.PageURL != vnePage || v.Title != "Tiêu đề video" {
		t.Fatalf("%+v", v)
	}
	for _, p := range c.Paragraphs() {
		if strings.Contains(p, "Channel 12") || strings.Contains(p, "Tiêu đề video") {
			t.Fatal("video text leaked into paragraphs")
		}
	}
	// Poster as a plain img inside box_img_video (seen on Thể thao pages).
	plain := strings.Replace(vneVideoFig, `<img class="thumb-above-video" src=`, `<img src=`, 1)
	if c, _ = ExtractArticle(vneBody(plain+`<p>`+long+`</p>`), "vnexpress", vnePage); !strings.HasPrefix(c.Blocks[2].Video.Poster, "https://iv1.vnecdn.net/") {
		t.Fatalf("plain poster: %+v", c.Blocks[2].Video)
	}
	// A media URL outside the verified host/path keeps the video as a link.
	c, _ = ExtractArticle(vneBody(strings.Replace(vneVideoFig, "https://d1.vnecdn.net/vnexpress/video/", "https://cdn.example/vnexpress/video/", 1)+`<p>`+long+`</p>`), "vnexpress", vnePage)
	if v := c.Blocks[2].Video; v.Kind != "link" || v.Src != "" || v.Poster == "" {
		t.Fatalf("unverified src: %+v", v)
	}
}

// bbcPage builds a BBC page whose window.__INITIAL_DATA__ holds the given
// data object, as on the real pages (JSON stored in a JS string).
func bbcDataPage(body string, data map[string]any) []byte {
	j, _ := json.Marshal(map[string]any{"data": data})
	s, _ := json.Marshal(string(j))
	return []byte(`<html><head><meta property="og:image" content="https://ichef.bbci.co.uk/ace/branded_news/1200/cpsprodpb/x/og.jpg"></head><body><article>` + body + `</article><script>window.__INITIAL_DATA__=` + string(s) + `;</script></body></html>`)
}

func bbcMediaBlock(kind, caption string, embed bool) map[string]any {
	return map[string]any{"type": "media", "model": map[string]any{
		"caption": map[string]any{"model": map[string]any{"blocks": []any{map[string]any{"model": map[string]any{"text": caption}}}}},
		"media": map[string]any{"__typename": "ElementsMediaPlayer", "items": []any{map[string]any{"id": "p0pd7ry4", "idType": "versionID", "title": caption, "duration": 89,
			"holdingImageUrl": "https://ichef.bbci.co.uk/images/ic/{width}xn/p0pd7m38.jpg", "kind": kind, "isEmbeddingAllowed": embed}},
			"externalEmbedUrl": "https://www.bbc.co.uk/ws/av-embeds/articles/x/{vpid}/en-GB/"}}}
}

func TestBBCVideoBlocks(t *testing.T) {
	text := `<div data-block="text"><p>` + long + `</p></div>`
	data := map[string]any{"article?x=1": map[string]any{"data": map[string]any{"content": map[string]any{"model": map[string]any{"blocks": []any{
		map[string]any{"type": "text"}, bbcMediaBlock("radioProgramme", "Podcast", true), bbcMediaBlock("programme", "Hope for new trial", true)}}}}}}
	c, e := ExtractArticle(bbcDataPage(text+`<div data-block="media"><figure>audio</figure></div>`+text+`<div data-block="media"><figure>video</figure></div>`+text, data), "bbc", bbcPage0)
	if e != nil {
		t.Fatal(e)
	}
	kinds := []string{}
	for _, b := range c.Blocks {
		kinds = append(kinds, b.Type)
	}
	if strings.Join(kinds, ",") != "p,p,video,p" {
		t.Fatalf("order %v", kinds)
	}
	v := c.Blocks[2].Video
	// Embedding is allowed for this item, but the embed was not verified to
	// play when framed, so BBC videos are links to the source.
	if v.Kind != "link" || v.Src != "" || v.Poster != "https://ichef.bbci.co.uk/images/ic/976xn/p0pd7m38.jpg" || v.Caption != "Hope for new trial" || v.Duration != 89 || v.PageURL != bbcPage0 {
		t.Fatalf("%+v", v)
	}
}

const bbcPage0 = "https://www.bbc.co.uk/news/articles/cme3r9ny2p4xo"

func TestBBCVideoPageIsVideoNotFullText(t *testing.T) {
	data := map[string]any{"media-experience?id=c3": map[string]any{"data": map[string]any{"initialItem": map[string]any{"mediaItem": map[string]any{
		"media": map[string]any{"__typename": "ElementsMediaPlayer", "items": []any{map[string]any{"title": "Why has Brazil accused the US?", "duration": 117, "kind": "programme",
			"holdingImage": map[string]any{"url": "https://ichef.bbci.co.uk/ace/standard/{width}/galileo/p0pdrj4q.jpg"}, "isEmbeddingAllowed": false}}}}}}}}
	c, e := ExtractArticle(bbcDataPage(`<div data-block="headline"><h1>Why</h1></div>`, data), "bbc", "https://www.bbc.co.uk/news/videos/c3eweld0nddeo")
	if e == nil {
		t.Fatal("video page marked full")
	}
	if len(c.Blocks) != 1 || c.Blocks[0].Video == nil || c.Blocks[0].Video.Poster != "https://ichef.bbci.co.uk/ace/standard/976/galileo/p0pdrj4q.jpg" || c.Blocks[0].Video.Kind != "link" {
		t.Fatalf("%+v", c.Blocks)
	}
	// Missing or broken page data: no video, no panic.
	for _, b := range []string{`<article></article><script>window.__INITIAL_DATA__="{bad";</script>`, `<article></article><script>window.__INITIAL_DATA__="null";</script>`} {
		if c, _ := ExtractArticle([]byte(b), "bbc", "https://www.bbc.co.uk/news/videos/x"); c.HasVideo() {
			t.Fatal("video from broken data")
		}
	}
}

func TestTuoiTreVideoPage(t *testing.T) {
	ld := `<script type="application/ld+json">{"@context":"https://schema.org","@type":"VideoObject","name":"Công an TP.HCM truy quét hàng giả","thumbnailUrl":["https://cdn2.tuoitre.vn/thumb_w/1200/a.jpg"],"duration":"PT5M14.520S","contentUrl":"https://cdn2.tuoitre.vn/471584752817336320/2026/10/5/a.mp4"}</script>`
	page := "https://tuoitre.vn/video/cong-an-tphcm-203229.htm"
	c, e := ExtractArticle(ttDoc(`<meta property="og:type" content="website">`+ld, ttAuthor("ĐAN THUẦN"), ""), "tuoitre", page)
	if e == nil || len(c.Blocks) != 1 {
		t.Fatalf("%v %+v", e, c.Blocks)
	}
	v := c.Blocks[0].Video
	if v.Kind != "mp4" || v.Src != "https://cdn2.tuoitre.vn/471584752817336320/2026/10/5/a.mp4" || v.Poster != "https://cdn2.tuoitre.vn/thumb_w/1200/a.jpg" || v.Duration != 314 || v.PageURL != page {
		t.Fatalf("%+v", v)
	}
	// Raw line breaks inside a JSON string (as on a real page) still parse.
	broken := strings.Replace(ld, `"name":"Công an TP.HCM truy quét hàng giả"`, "\"name\":\"Công an\n\ntruy quét\",\n\"description\":\"Dòng 1.\n\n\"", 1)
	if c, _ = ExtractArticle(ttDoc(`<meta property="og:type" content="website">`+broken, "", ""), "tuoitre", page); !c.HasVideo() || c.Blocks[0].Video.Kind != "mp4" {
		t.Fatalf("line breaks in JSON-LD: %+v", c.Blocks)
	}
	// contentUrl on another host: link only.
	c, _ = ExtractArticle(ttDoc(strings.Replace(ld, "https://cdn2.tuoitre.vn/4715", "https://other.example/4715", 1), "", ""), "tuoitre", page)
	if v := c.Blocks[0].Video; v.Kind != "link" || v.Src != "" {
		t.Fatalf("%+v", v)
	}
}

func TestISODuration(t *testing.T) {
	for in, want := range map[string]int{"PT1M53S": 113, "PT5M14.520S": 314, "PT1H": 3600, "PT45S": 45, "": 0, "5 min": 0} {
		if got := isoSeconds(in); got != want {
			t.Errorf("%q: %d", in, got)
		}
	}
}

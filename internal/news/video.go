package news

import (
	"encoding/json"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// Video is a video taken from the article page. The browser plays Src straight
// from the source CDN; nothing is downloaded, stored or proxied by the server.
//
// Kind, decided per source from what was verified on 2026-10-05:
//   - "mp4":  Tuổi Trẻ /video/ pages, VideoObject.contentUrl on cdn2.tuoitre.vn
//     (public, byte ranges, CORS *, no token).
//   - "link": not playable here. Only the poster, caption and a link to the
//     source page are shown.
//     BBC: the official embed did not render when framed.
//     VnExpress: the HLS playlists on d1.vnecdn.net answered 200 at first,
//     but on re-check (2026-10-05 12:09 UTC) they answered 405/406/407
//     "forbidden" unless the Referer is vnexpress.net: hotlink protection,
//     which this app does not work around.
//     Tuổi Trẻ videos inside articles: media URL not verified.
type Video struct {
	Kind     string `json:"kind"`
	Src      string `json:"src,omitempty"`
	Poster   string `json:"poster,omitempty"`
	Width    int    `json:"width,omitempty"`
	Height   int    `json:"height,omitempty"`
	Duration int    `json:"duration,omitempty"` // seconds
	Title    string `json:"title,omitempty"`
	Caption  string `json:"caption,omitempty"`
	Credit   string `json:"credit,omitempty"`
	PageURL  string `json:"page_url"` // the article page that carries the player
}

// VideoSrc accepts a media URL only for the kind and host/path verified for it.
func VideoSrc(kind, raw string) (string, bool) {
	u, e := url.Parse(strings.TrimSpace(raw))
	if e != nil || u.Scheme != "https" || u.User != nil || (u.Port() != "" && u.Port() != "443") || u.RawQuery != "" || u.Fragment != "" {
		return "", false
	}
	h := strings.ToLower(u.Hostname())
	switch kind {
	case "mp4":
		if h != ttImageHost || !strings.HasSuffix(u.Path, ".mp4") || strings.HasPrefix(u.Path, "/1.1/") {
			return "", false
		}
	default:
		return "", false
	}
	u.Host = h
	return u.String(), true
}

// validVideo enforces the Kind/Src contract before a block is stored.
func validVideo(v *Video) *Video {
	if v == nil || v.PageURL == "" {
		return nil
	}
	if v.Kind != "link" {
		src, ok := VideoSrc(v.Kind, v.Src)
		if !ok {
			// Unverified media URL: keep the video as a link to the source.
			v.Kind, src = "link", ""
		}
		v.Src = src
	} else {
		v.Src = ""
	}
	if v.Width <= 0 || v.Height <= 0 || v.Width > 20000 || v.Height > 20000 {
		v.Width, v.Height = 0, 0
	}
	if v.Duration < 0 || v.Duration > 24*3600 {
		v.Duration = 0
	}
	return v
}

func posterURL(raw string, base *url.URL) string {
	u, _ := ImageURL(raw, base)
	return u
}

var vneVideoCredit = regexp.MustCompile(`(?i)\s*(Video|Nguồn)\s*:\s*`)

// vneVideo reads a VnExpress embedded video as a link to the article (see
// Video): box_embed_video_parent with data-vwidth/vheight/duration, the
// poster img.thumb-above-video and the caption "Caption. Video: Source" in
// the figure's desc_cation. The playlist URL is not stored.
func vneVideo(fig *html.Node, base *url.URL, page string) *Video {
	box := findNode(fig, func(x *html.Node) bool { return hasClass(x, "box_embed_video_parent") })
	if box == nil {
		return nil
	}
	v := &Video{Kind: "link", PageURL: page, Width: dim(attr(box, "data-vwidth")), Height: dim(attr(box, "data-vheight"))}
	v.Duration, _ = strconv.Atoi(attr(box, "data-duration"))
	// Poster: img.thumb-above-video, or the plain img of box_img_video.
	img := findNode(box, func(x *html.Node) bool { return x.Data == "img" && hasClass(x, "thumb-above-video") })
	if img == nil {
		if thumb := findNode(box, func(x *html.Node) bool { return hasClass(x, "box_img_video") }); thumb != nil {
			img = findNode(thumb, func(x *html.Node) bool { return x.Data == "img" })
		}
	}
	if img != nil {
		v.Poster = posterURL(attr(img, "src"), base)
	}
	if t := findNode(fig, func(x *html.Node) bool { return hasClass(x, "parser_title") }); t != nil {
		v.Title = text(t)
	}
	if cap := findNode(fig, func(x *html.Node) bool { return x.Data == "figcaption" || hasClass(x, "desc_cation") }); cap != nil {
		s := text(cap)
		if l := vneVideoCredit.FindAllStringIndex(s, -1); len(l) > 0 {
			last := l[len(l)-1]
			v.Caption, v.Credit = strings.TrimSpace(s[:last[0]]), strings.TrimSpace(s[last[1]:])
		} else {
			v.Caption = s
		}
	}
	return validVideo(v)
}

// bbcInitialData decodes window.__INITIAL_DATA__ (a JSON document stored in a
// JavaScript string) from a BBC page.
var bbcDataRe = regexp.MustCompile(`window\.__INITIAL_DATA__=("(?:[^"\\]|\\.)*")`)

func bbcInitialData(page []byte) map[string]any {
	m := bbcDataRe.FindSubmatch(page)
	if m == nil {
		return nil
	}
	var s string
	if json.Unmarshal(m[1], &s) != nil {
		return nil
	}
	var d map[string]any
	if json.Unmarshal([]byte(s), &d) != nil {
		return nil
	}
	data, _ := d["data"].(map[string]any)
	return data
}

func path(o any, keys ...string) any {
	for _, k := range keys {
		m, ok := o.(map[string]any)
		if !ok {
			return nil
		}
		o = m[k]
	}
	return o
}

func str(o any, keys ...string) string { s, _ := path(o, keys...).(string); return s }

// bbcVideo builds a link-only video from an ElementsMediaPlayer item. Only
// video programmes are kept (not radio/audio).
func bbcVideo(media any, caption, page string, base *url.URL) *Video {
	items, _ := path(media, "items").([]any)
	if len(items) == 0 {
		return nil
	}
	it := items[0]
	if str(it, "kind") != "programme" {
		return nil
	}
	v := &Video{Kind: "link", PageURL: page, Title: str(it, "title"), Caption: caption}
	if d, ok := path(it, "duration").(float64); ok {
		v.Duration = int(d)
	}
	poster := str(it, "holdingImageUrl")
	if poster == "" {
		poster = str(it, "holdingImage", "url")
	}
	poster = strings.NewReplacer("{width}", "976", "$recipe", "976x549").Replace(poster)
	v.Poster = posterURL(poster, base)
	return validVideo(v)
}

// bbcArticleVideos lists, in reading order, the video of every
// data-block="media" of a BBC article (nil entries keep the order for media
// that are not videos).
func bbcArticleVideos(data map[string]any, page string, base *url.URL) []*Video {
	var out []*Video
	for k, v := range data {
		if !strings.HasPrefix(k, "article?") {
			continue
		}
		blocks, _ := path(v, "data", "content", "model", "blocks").([]any)
		for _, b := range blocks {
			if str(b, "type") != "media" {
				continue
			}
			caption := ""
			if cb, _ := path(b, "model", "caption", "model", "blocks").([]any); len(cb) > 0 {
				caption = str(cb[0], "model", "text")
			}
			out = append(out, bbcVideo(path(b, "model", "media"), caption, page, base))
		}
	}
	return out
}

// bbcVideoPage returns the main video of a BBC short-form video page
// (/news/videos/…), whose player is described under "media-experience".
func bbcVideoPage(data map[string]any, page string, base *url.URL) *Video {
	for k, v := range data {
		if strings.HasPrefix(k, "media-experience") {
			item := path(v, "data", "initialItem", "mediaItem")
			return bbcVideo(path(item, "media"), "", page, base)
		}
	}
	return nil
}

// ttVideoPage reads the VideoObject JSON-LD of a Tuổi Trẻ /video/ page.
func ttVideoPage(root *html.Node, base *url.URL, page string) *Video {
	var v *Video
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if v != nil {
			return
		}
		if n.Type == html.ElementNode && n.Data == "script" && attr(n, "type") == "application/ld+json" && n.FirstChild != nil {
			// Some pages put raw line breaks inside JSON strings (invalid JSON,
			// seen 2026-10-05); outside strings they are plain whitespace.
			raw := strings.NewReplacer("\n", " ", "\r", " ", "\t", " ").Replace(n.FirstChild.Data)
			var o map[string]any
			if json.Unmarshal([]byte(raw), &o) == nil && o["@type"] == "VideoObject" {
				v = &Video{Kind: "mp4", PageURL: page, Src: str(o, "contentUrl"), Title: ttNorm(str(o, "name"))}
				thumb := str(o, "thumbnailUrl")
				if l, ok := o["thumbnailUrl"].([]any); ok && len(l) > 0 {
					thumb, _ = l[0].(string)
				}
				v.Poster = posterURL(thumb, base)
				v.Duration = isoSeconds(str(o, "duration"))
			}
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	return validVideo(v)
}

// ttInlineVideo: a VideoStream box inside a Tuổi Trẻ article. Its media URL
// is only known in the provider's player URL, which is not used; the poster
// it names is kept with a link to the article.
func ttInlineVideo(n *html.Node, base *url.URL, page string) *Video {
	v := &Video{Kind: "link", PageURL: page}
	if u, e := url.Parse(attr(n, "data-src")); e == nil {
		v.Poster = posterURL(u.Query().Get("poster"), base)
	}
	return validVideo(v)
}

var isoDuration = regexp.MustCompile(`^PT(?:(\d+)H)?(?:(\d+)M)?(?:(\d+)(?:\.\d+)?S)?$`)

func isoSeconds(s string) int {
	m := isoDuration.FindStringSubmatch(s)
	if m == nil {
		return 0
	}
	n := 0
	for i, mul := range []int{3600, 60, 1} {
		x, _ := strconv.Atoi(m[i+1])
		n += x * mul
	}
	return n
}

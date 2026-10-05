package news

import (
	"errors"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html"
	"golang.org/x/text/unicode/norm"
)

// Tuổi Trẻ Online (tuoitre.vn) article pages, structure checked on 2026-10-05
// against news pages of Thời sự, Thế giới and Kinh doanh, a series article and
// a /video/ page:
//
//   - Body: the element with data-role="content" (class detail-content). The
//     "content fck" element elsewhere on the page is an empty popup holder.
//   - Body children are paragraphs, h2 subheadings and CMS boxes marked with a
//     type attribute: "Photo" figures (img src = 730px rendition, data-original
//     = full file, w/h = size; caption in figcaption.PhotoCMS_Caption, credit
//     as "... - Ảnh: X"), "content" boxes holding article text, and related
//     links ("RelatedNewsBox", "RelatedOneNews") that are not article content.
//   - Byline: div.detail-author[data-role=author], one .author-item-name link
//     per author. meta author is only "TUOI TRE ONLINE" and is not used.
//   - Video pages have og:type "website" and no paragraphs.

// ttImageHost is the only CDN seen serving article-body images.
const ttImageHost = "cdn2.tuoitre.vn"

// ttBodyBox are CMS box types whose content is article text.
var ttBodyBox = map[string]bool{"content": true}

// ttOrgNames are desks or the publication itself, not people.
var ttOrgNames = map[string]bool{"tuoi tre online": true, "tuổi trẻ online": true, "tuổi trẻ": true, "tto": true, "media": true}

// ttSpace removes the space text() leaves before punctuation that follows an
// inline link or emphasis ("Ngoại thương , có" -> "Ngoại thương, có").
var ttSpace = regexp.MustCompile(`\s+([,.;:!?)”])`)

// Tuổi Trẻ pages mix composed and decomposed Vietnamese (e.g. "A" + U+0301
// in bylines). Text is stored composed (NFC) so search and comparisons work.
func ttText(n *html.Node) string { return ttSpace.ReplaceAllString(norm.NFC.String(text(n)), "$1") }

func ttNorm(s string) string { return norm.NFC.String(strings.TrimSpace(s)) }

// PlainIn is Plain for feed fields of a given source.
func PlainIn(s, adapter string) string {
	if adapter == "tuoitre" {
		return norm.NFC.String(Plain(s))
	}
	return Plain(s)
}

var ttCredit = regexp.MustCompile(`\s*[-–]?\s*\(?\s*Ảnh\s*:\s*`)

// splitTTCredit splits "Caption - Ảnh: REUTERS" into caption and credit. Other
// notes such as "Ảnh cắt từ clip" stay in the caption.
func splitTTCredit(s string) (string, string) {
	locs := ttCredit.FindAllStringIndex(s, -1)
	if len(locs) == 0 {
		return s, ""
	}
	l := locs[len(locs)-1]
	return strings.TrimSpace(s[:l[0]]), strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s[l[1]:]), ")"))
}

func ttImage(fig *html.Node, base *url.URL) *Image {
	img := findNode(fig, func(x *html.Node) bool { return x.Data == "img" })
	if img == nil {
		return nil
	}
	src, ok := ImageURL(attr(img, "src"), base)
	if !ok {
		return nil
	}
	im := &Image{Src: src, Alt: norm.NFC.String(strings.TrimSpace(attr(img, "alt")))}
	w, h := dim(attr(img, "w")), dim(attr(img, "h"))
	if w == 0 || h == 0 {
		w, h = dim(attr(img, "width")), dim(attr(img, "height"))
	}
	if w > 0 && h > 0 {
		im.Width, im.Height = w, h
		// The rendition in src is 730px wide; the full file serves larger screens.
		if full, ok := ImageURL(attr(img, "data-original"), base); ok && w > 730 {
			im.Srcset = src + " 730w, " + full + " " + strconv.Itoa(w) + "w"
		}
	}
	if fc := findNode(fig, func(x *html.Node) bool { return x.Data == "figcaption" || hasClass(x, "PhotoCMS_Caption") }); fc != nil {
		im.Caption, im.Credit = splitTTCredit(ttText(fc))
	}
	return im
}

// ttAuthors reads the byline names; the avatar, "và N tác giả khác" and the
// organisation are not authors.
func ttAuthors(root *html.Node) []string {
	box := findNode(root, func(n *html.Node) bool { return hasClass(n, "detail-author") && attr(n, "data-role") == "author" })
	if box == nil {
		return nil
	}
	out := []string{}
	add := func(a *html.Node) {
		name := norm.NFC.String(strings.TrimSpace(attr(a, "title")))
		if name == "" {
			name = ttText(a)
		}
		if n := utf8.RuneCountInString(name); n >= 2 && n <= 60 && !ttOrgNames[strings.ToLower(name)] {
			out = append(out, name)
		}
	}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && hasClass(n, "author-item-name") {
			if a := findNode(n, func(x *html.Node) bool { return x.Data == "a" }); a != nil {
				add(a)
			}
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(box)
	if len(out) == 0 {
		if a := findNode(box, func(x *html.Node) bool { return hasClass(x, "author-info") }); a != nil {
			if n := findNode(a, func(x *html.Node) bool { return x.Data == "a" && hasClass(x, "name") }); n != nil {
				add(n)
			}
		}
	}
	return dedupe(out)
}

func metaProperty(root *html.Node, prop string) string {
	m := findNode(root, func(n *html.Node) bool { return n.Data == "meta" && attr(n, "property") == prop })
	return attrOf(m, "content")
}

// extractTuoiTre is the Tuổi Trẻ article extractor (see the notes above).
func extractTuoiTre(root *html.Node, base *url.URL, c Content) (Content, error) {
	c.Authors = ttAuthors(root)
	if strings.HasPrefix(base.Path, "/video/") || metaProperty(root, "og:type") == "website" {
		if v := ttVideoPage(root, base, base.String()); v != nil {
			c.Blocks = []Block{{Type: "video", Video: v}}
		}
		return c, errors.New("trang video, không có toàn văn dạng chữ")
	}
	scope := findNode(root, func(n *html.Node) bool { return attr(n, "data-role") == "content" && hasClass(n, "detail-content") })
	if scope == nil {
		return c, errors.New("chưa tìm thấy vùng nội dung bài")
	}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "script", "style", "noscript", "nav", "aside", "footer", "form", "button", "iframe", "video", "svg":
				return
			}
			if t := attr(n, "type"); t != "" {
				switch {
				case strings.EqualFold(t, "photo") && n.Data == "figure":
					if im := ttImage(n, base); im != nil {
						c.Blocks = append(c.Blocks, Block{Type: "img", Image: im})
					}
					return
				case t == "VideoStream":
					if v := ttInlineVideo(n, base, base.String()); v != nil {
						c.Blocks = append(c.Blocks, Block{Type: "video", Video: v})
					}
					return
				case !ttBodyBox[t]:
					// Related links, videos, polls and other widgets.
					return
				}
			}
			if hasClass(n, "readmore-body-box") || n.Data == "figure" || n.Data == "figcaption" {
				return
			}
			switch n.Data {
			case "p", "h2", "h3":
				if s := ttText(n); s != "" {
					c.Blocks = append(c.Blocks, Block{Type: "p", Text: s})
				}
				return
			}
		}
		for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
			walk(ch)
		}
	}
	walk(scope)
	return c, nil
}

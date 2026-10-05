package news

import (
	"bytes"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html"
)

// ExtractVersion identifies the extractor that produced stored content.
// 0: rows stored before media support; 2: first media build (could keep
// video poster frames and crashed on some photo pages, never released);
// 3: blocks with images, authors and lead image; 4: video blocks. Rows below
// the current version are re-read once by the enrichment backfill.
const ExtractVersion = 4

// Image is an image taken from the article itself. Alt, caption and credit are
// copied from the source page; nothing is generated.
type Image struct {
	Src     string `json:"src"`
	Srcset  string `json:"srcset,omitempty"`
	Width   int    `json:"width,omitempty"`
	Height  int    `json:"height,omitempty"`
	Alt     string `json:"alt"`
	Caption string `json:"caption,omitempty"`
	Credit  string `json:"credit,omitempty"`
}

// Block is one piece of the article body in reading order: a paragraph ("p"),
// an image ("img") or a video ("video").
type Block struct {
	Type  string `json:"type"`
	Text  string `json:"text,omitempty"`
	Image *Image `json:"image,omitempty"`
	Video *Video `json:"video,omitempty"`
}

type Content struct {
	Blocks  []Block
	Authors []string
	Lead    *Image // og:image; shown only when the body has no image
}

func (c Content) Paragraphs() []string {
	out := []string{}
	for _, b := range c.Blocks {
		if b.Type == "p" {
			out = append(out, b.Text)
		}
	}
	return out
}

// HasVideo reports a video block. It says what the article contains, not that
// the video can be played or that the text is complete.
func (c Content) HasVideo() bool {
	for _, b := range c.Blocks {
		if b.Type == "video" {
			return true
		}
	}
	return false
}

// VideoBlocks keeps only the video blocks (stored for pages without full text).
func (c Content) VideoBlocks() []Block {
	out := []Block{}
	for _, b := range c.Blocks {
		if b.Type == "video" {
			out = append(out, b)
		}
	}
	return out
}
func (c Content) Images() int {
	n := 0
	for _, b := range c.Blocks {
		if b.Type == "img" {
			n++
		}
	}
	return n
}

// ImageHosts are the CDNs the browser may load images from; they are also
// listed in the Content-Security-Policy img-src.
func imageHostAllowed(h string) bool {
	return h == "ichef.bbci.co.uk" || strings.HasSuffix(h, ".vnecdn.net") || h == ttImageHost
}

// ImageURL resolves raw against base and accepts only HTTPS URLs on the
// verified image CDNs.
func ImageURL(raw string, base *url.URL) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.HasPrefix(raw, "data:") {
		return "", false
	}
	u, e := base.Parse(raw)
	if e != nil || u.Scheme != "https" || u.User != nil || (u.Port() != "" && u.Port() != "443") {
		return "", false
	}
	u.Host = strings.ToLower(u.Hostname())
	if !imageHostAllowed(u.Host) {
		return "", false
	}
	u.Fragment = ""
	return u.String(), true
}

var srcsetDescriptor = regexp.MustCompile(`^\d+(\.\d+)?[wx]$`)

// cleanSrcset keeps only srcset candidates whose URL passes ImageURL.
func cleanSrcset(v string, base *url.URL) string {
	out := []string{}
	for _, c := range strings.Split(v, ",") {
		f := strings.Fields(c)
		if len(f) != 2 || !srcsetDescriptor.MatchString(f[1]) {
			continue
		}
		if u, ok := ImageURL(f[0], base); ok {
			out = append(out, u+" "+f[1])
		}
	}
	return strings.Join(out, ", ")
}

func dim(s string) int {
	f, e := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if e != nil || f < 1 || f > 20000 {
		return 0
	}
	return int(f + 0.5)
}

// imageFrom builds an Image from the first <img> under n. Lazy-loaded
// VnExpress images keep the real URL in data-src / data-srcset; src may be a
// placeholder.
func imageFrom(n *html.Node, base *url.URL) *Image {
	if n == nil {
		return nil
	}
	img := findNode(n, func(x *html.Node) bool { return x.Data == "img" })
	var src string
	ok := false
	for _, cand := range []string{attr(n, "data-src"), attrOf(img, "data-src"), attrOf(img, "src")} {
		if src, ok = ImageURL(cand, base); ok {
			break
		}
	}
	if !ok {
		return nil
	}
	im := &Image{Src: src, Alt: strings.TrimSpace(attrOf(img, "alt"))}
	for _, s := range []string{attrOf(img, "data-srcset"), attrOf(img, "srcset")} {
		if im.Srcset = cleanSrcset(s, base); im.Srcset != "" {
			break
		}
	}
	if im.Srcset == "" {
		// <source> without type, or a JPEG/PNG one; WebP is offered by <img srcset> on BBC.
		if s := findNode(n, func(x *html.Node) bool { return x.Data == "source" && !strings.Contains(attr(x, "type"), "webp") }); s != nil {
			im.Srcset = cleanSrcset(attr(s, "data-srcset")+","+attr(s, "srcset"), base)
		}
	}
	im.Width, im.Height = dim(attrOf(img, "width")), dim(attrOf(img, "height"))
	if im.Width == 0 || im.Height == 0 {
		w := findNode(n, func(x *html.Node) bool { return x.Data == "meta" && attr(x, "itemprop") == "width" })
		h := findNode(n, func(x *html.Node) bool { return x.Data == "meta" && attr(x, "itemprop") == "height" })
		if w != nil && h != nil {
			im.Width, im.Height = dim(attr(w, "content")), dim(attr(h, "content"))
		}
	}
	if im.Width == 0 || im.Height == 0 {
		im.Width, im.Height = 0, 0
	}
	return im
}
func attrOf(n *html.Node, k string) string {
	if n == nil {
		return ""
	}
	return attr(n, k)
}

// splitVnECredit splits "Caption. Ảnh: AFP" into caption and credit.
var vneCredit = regexp.MustCompile(`(?i)\s*(Ảnh|Ảnh minh họa|Ảnh minh hoạ)\s*:\s*`)

func splitVnECredit(s string) (string, string) {
	locs := vneCredit.FindAllStringIndex(s, -1)
	if len(locs) == 0 {
		return s, ""
	}
	l := locs[len(locs)-1]
	return strings.TrimSpace(s[:l[0]]), strings.TrimSpace(s[l[1]:])
}

func vneImage(n *html.Node, base *url.URL) *Image {
	im := imageFrom(n, base)
	if im == nil {
		return nil
	}
	cap := findNode(n, func(x *html.Node) bool { return x.Data == "figcaption" || hasClass(x, "Image") })
	if cap != nil {
		im.Caption, im.Credit = splitVnECredit(text(cap))
	}
	return im
}

func bbcImage(n *html.Node, base *url.URL) *Image {
	im := imageFrom(n, base)
	if im == nil {
		return nil
	}
	if fc := findNode(n, func(x *html.Node) bool { return x.Data == "figcaption" }); fc != nil {
		if p := findNode(fc, func(x *html.Node) bool { return x.Data == "p" }); p != nil {
			im.Caption = text(p)
		}
	}
	if cr := findNode(n, func(x *html.Node) bool { return x.Data == "span" && attr(x, "role") == "text" }); cr != nil {
		im.Credit = strings.TrimSpace(strings.TrimPrefix(text(cr), "Image source,"))
	}
	// Newsletter and promo banners are image blocks without caption or credit,
	// very wide (6:1 and more on real pages) and labelled "banner" in their alt.
	if im.Caption == "" && im.Credit == "" {
		wide := im.Height > 0 && im.Width >= 4*im.Height
		if wide || strings.Contains(strings.ToLower(im.Alt), "banner") {
			return nil
		}
	}
	return im
}

// leadImage reads og:image from the page head.
func leadImage(root *html.Node, base *url.URL) *Image {
	var src, alt string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "meta" {
			switch attr(n, "property") {
			case "og:image":
				if src == "" {
					src = attr(n, "content")
				}
			case "og:image:alt":
				alt = attr(n, "content")
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	if u, ok := ImageURL(src, base); ok {
		return &Image{Src: u, Alt: strings.TrimSpace(alt)}
	}
	return nil
}

func rightAligned(n *html.Node) bool {
	st := strings.ReplaceAll(strings.ToLower(attr(n, "style")), " ", "")
	return strings.Contains(st, "text-align:right") || strings.EqualFold(attr(n, "align"), "right")
}

var authorDropLabels = map[string]bool{"ảnh": true, "thiết kế": true, "đồ họa": true, "đồ hoạ": true, "video": true, "kỹ thuật": true, "dựng": true, "quay": true}

// parseVnEAuthors reads a VnExpress sign-off line such as "Phương Thảo (theo
// Robb Report)", "A - B", "Nội dung: A - Thiết kế: B" or "Việt Thành tổng hợp".
func parseVnEAuthors(p *html.Node) []string {
	s := ""
	if b := findNode(p, func(x *html.Node) bool { return x.Data == "strong" || x.Data == "b" }); b != nil {
		s = text(b)
	}
	if s == "" {
		s = text(p)
	}
	if i := strings.Index(s, "("); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimSpace(s)
	if strings.HasPrefix(strings.ToLower(s), "theo ") { // attribution only, no author
		return nil
	}
	out := []string{}
	for _, seg := range regexp.MustCompile(`\s+-\s+|,\s*|\s+và\s+|\s*&\s*`).Split(s, -1) {
		if label, name, ok := strings.Cut(seg, ":"); ok {
			if authorDropLabels[strings.ToLower(strings.TrimSpace(label))] {
				continue
			}
			seg = name
		}
		seg = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(seg), " tổng hợp"))
		if n := utf8.RuneCountInString(seg); n >= 2 && n <= 60 && !strings.EqualFold(seg, "VnExpress") {
			out = append(out, seg)
		}
	}
	return out
}

// vneSignature finds the sign-off paragraph: the last text paragraph of the
// body, right-aligned and short. Anything else (quotes, people named in the
// text, comments) is never treated as an author.
func vneSignature(scope *html.Node) (*html.Node, []string) {
	var last *html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			if attr(n, "data-component") != "" || n.Data == "figure" || hasClass(n, "tplCaption") || n.Data == "script" {
				return
			}
			if n.Data == "p" && text(n) != "" {
				last = n
				return
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(scope)
	if last == nil || !rightAligned(last) || utf8.RuneCountInString(text(last)) > 150 {
		return nil, nil
	}
	a := parseVnEAuthors(last)
	if len(a) == 0 {
		return nil, nil
	}
	return last, a
}

// bbcAuthors reads the byline block: "By <name>" with an optional role, or a
// multi-byline "By A and B, BBC News Investigations". Roles, organisations and
// the avatar are not authors.
func bbcAuthors(scope *html.Node) []string {
	by := findNode(scope, func(n *html.Node) bool {
		t := attr(n, "data-testid")
		return t == "single-byline" || t == "multi-byline"
	})
	if by == nil {
		return nil
	}
	isBy := func(n *html.Node) bool { return n.Type == html.ElementNode && strings.EqualFold(text(n), "By") }
	out := []string{}
	if attr(by, "data-testid") == "single-byline" {
		b := findNode(by, isBy)
		if b == nil {
			return nil
		}
		for s := b.NextSibling; s != nil; s = s.NextSibling {
			if s.Type == html.ElementNode {
				if t := text(s); t != "" {
					out = append(out, t)
				}
				break
			}
		}
		return out
	}
	hidden := func(n *html.Node) bool {
		return findNode(n, func(x *html.Node) bool { return attr(x, "aria-hidden") == "true" }) != nil
	}
	for c := by.FirstChild; c != nil; c = c.NextSibling {
		if c.Type != html.ElementNode || isBy(c) {
			continue
		}
		for s := c.FirstChild; s != nil; s = s.NextSibling {
			if s.Type != html.ElementNode {
				continue
			}
			t := text(s)
			if t == "" || strings.EqualFold(t, "and") || strings.HasPrefix(t, ",") || hidden(s) {
				continue
			}
			out = append(out, t)
			break
		}
	}
	return out
}

func dedupe(in []string) []string {
	seen, out := map[string]bool{}, []string{}
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// ExtractArticle returns the article body as ordered blocks plus authors and
// the lead image. On error (no recognised body, too little text, interactive
// page) Content still carries the authors and lead image that were found.
func ExtractArticle(b []byte, adapter, pageURL string) (c Content, err error) {
	// A page with an unexpected structure must fail this one article, never
	// the worker or the server.
	defer func() {
		if r := recover(); r != nil {
			c, err = Content{}, fmt.Errorf("lỗi xử lý cấu trúc trang: %v", r)
		}
	}()
	base, e := url.Parse(pageURL)
	if e != nil {
		return c, e
	}
	root, e := html.Parse(bytes.NewReader(b))
	if e != nil {
		return c, e
	}
	c.Lead = leadImage(root, base)
	page := base.String()
	if adapter == "tuoitre" {
		if c, e = extractTuoiTre(root, base, c); e != nil {
			return c, e
		}
		return checkFull(adapter, c)
	}
	var scope *html.Node
	switch adapter {
	case "vnexpress":
		scope = findNode(root, func(n *html.Node) bool { return hasClass(n, "fck_detail") && n.Data != "style" })
	case "bbc":
		scope = findNode(root, func(n *html.Node) bool { return n.Data == "article" })
	}
	if scope == nil {
		return c, errors.New("chưa tìm thấy vùng nội dung bài")
	}
	var signature *html.Node
	switch adapter {
	case "vnexpress":
		signature, c.Authors = vneSignature(scope)
		if w := findNode(scope, func(n *html.Node) bool { return interactiveVnExpress[attr(n, "data-component-type")] }); w != nil {
			return c, fmt.Errorf("bài dạng tương tác (%s), không có toàn văn dạng chữ", attr(w, "data-component-type"))
		}
	case "bbc":
		c.Authors = bbcAuthors(scope)
	}
	c.Authors = dedupe(c.Authors)
	addImg := func(im *Image) {
		if im != nil {
			c.Blocks = append(c.Blocks, Block{Type: "img", Image: im})
		}
	}
	addVideo := func(v *Video) {
		if v != nil {
			c.Blocks = append(c.Blocks, Block{Type: "video", Video: v})
		}
	}
	var bbcVideos []*Video
	var bbcData map[string]any
	if adapter == "bbc" {
		bbcData = bbcInitialData(b)
		bbcVideos = bbcArticleVideos(bbcData, page, base)
	}
	media := 0
	addText := func(n *html.Node) {
		s := text(n)
		promo := adapter == "bbc" && findNode(n, func(a *html.Node) bool {
			return a.Data == "a" && strings.Contains(attr(a, "href"), "/newsletters/")
		}) != nil
		related := adapter == "vnexpress" && strings.HasPrefix(s, ">>")
		if s != "" && !promo && !related && n != signature {
			c.Blocks = append(c.Blocks, Block{Type: "p", Text: s})
		}
	}
	var walk func(n *html.Node, body bool)
	walk = func(n *html.Node, body bool) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "script", "style", "noscript", "nav", "aside", "footer", "form", "button", "iframe":
				return
			}
			if adapter == "vnexpress" {
				if attr(n, "data-component") != "" {
					return
				}
				// Embedded videos (also wrapped in item_slide_show figures): a
				// video block; the poster frame is never an article photo.
				if hasClass(n, "box_embed_video_parent") || findNode(n, func(x *html.Node) bool { return hasClass(x, "box_embed_video_parent") }) != nil && (n.Data == "figure" || hasClass(n, "item_slide_show")) {
					addVideo(vneVideo(n, base, page))
					return
				}
				if hasClass(n, "item_slide_show") {
					// Photo story: the image, then its narration. The same
					// narration is repeated inside the image overlay; skip that copy.
					thumb := findNode(n, func(x *html.Node) bool { return hasClass(x, "block_thumb_slide_show") })
					if thumb == nil {
						thumb = n
					}
					addImg(vneImage(thumb, base))
					for d := n.FirstChild; d != nil; d = d.NextSibling {
						if d.Type == html.ElementNode && hasClass(d, "desc_cation") {
							for p := d.FirstChild; p != nil; p = p.NextSibling {
								if p.Type == html.ElementNode && p.Data == "p" {
									addText(p)
								}
							}
						}
					}
					return
				}
				if n.Data == "figure" || hasClass(n, "tplCaption") {
					addImg(vneImage(n, base))
					return
				}
				if hasClass(n, "Image") || n.Data == "figcaption" {
					return
				}
			}
			if adapter == "bbc" {
				switch attr(n, "data-block") {
				case "text", "subheadline":
					body = true
				case "image":
					addImg(bbcImage(n, base))
					return
				case "media":
					// Matched in order with the media blocks of the page data.
					if media < len(bbcVideos) {
						addVideo(bbcVideos[media])
					}
					media++
					return
				case "":
				default:
					return
				}
				if n.Data == "figure" || n.Data == "figcaption" {
					return
				}
			}
			if body && (n.Data == "p" || (adapter == "bbc" && n.Data == "h2")) {
				addText(n)
				return
			}
		}
		for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
			walk(ch, body)
		}
	}
	walk(scope, adapter != "bbc")
	if adapter == "bbc" && !c.HasVideo() && strings.HasPrefix(base.Path, "/news/videos/") {
		if v := bbcVideoPage(bbcData, page, base); v != nil {
			c.Blocks = append([]Block{{Type: "video", Video: v}}, c.Blocks...)
		}
	}
	return checkFull(adapter, c)
}

// checkFull drops a lead image that repeats the body and refuses to call the
// page full text when it holds too little text (teaser, video, gallery).
func checkFull(adapter string, c Content) (Content, error) {
	paras := c.Paragraphs()
	total := 0
	for _, p := range paras {
		total += utf8.RuneCountInString(p)
	}
	c.Lead = dedupeLead(adapter, c.Lead, c.Blocks)
	if len(paras) < 2 || total < minFullChars {
		return c, errors.New("nội dung không đủ hoặc cấu trúc trang đã thay đổi")
	}
	return c, nil
}

var bbcBranded = regexp.MustCompile(`^/ace/branded_[a-z]+/`)

// dedupeLead drops the og:image when it would repeat an image of the body.
//   - BBC serves the same file under several renditions; og:image is the
//     "branded" one (BBC logo overlay), so it is switched to the "standard"
//     rendition used in article bodies and compared by file name.
//   - VnExpress og:image is a separate crop file that cannot be matched to a
//     body image, so it is kept only when the body has no image at all.
func dedupeLead(adapter string, lead *Image, blocks []Block) *Image {
	if lead == nil {
		return nil
	}
	hasImg := false
	for _, b := range blocks {
		if b.Type == "img" {
			hasImg = true
		}
	}
	switch adapter {
	case "bbc":
		u, e := url.Parse(lead.Src)
		if e != nil {
			return nil
		}
		u.Path = bbcBranded.ReplaceAllString(u.Path, "/ace/standard/")
		lead.Src = u.String()
		file := u.Path[strings.LastIndex(u.Path, "/")+1:]
		for _, b := range blocks {
			if b.Type == "img" && strings.Contains(b.Image.Src, "/"+file) {
				return nil
			}
		}
		return lead
	default:
		if hasImg {
			return nil
		}
		return lead
	}
}

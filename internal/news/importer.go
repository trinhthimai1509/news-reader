package news

import (
	"net/url"
	"regexp"
	"strings"
)

// CatalogFeed is a feed this project has checked (GET 200, parsed) and knows
// the name and category of. Section is the source's own page for the feed,
// taken from the feed's <channel><link> (2026-10-05); "" when the source does
// not declare one (VnExpress feeds point at themselves), in which case a
// section page URL is not mapped to the feed.
type CatalogFeed struct {
	Adapter  string
	FeedURL  string
	Name     string
	Category string
	Section  string
}

// Catalog: the official feeds already configured and checked in this project.
// Importing a home page adds these; nothing else is guessed.
var Catalog = []CatalogFeed{
	{"vnexpress", "https://vnexpress.net/rss/tin-moi-nhat.rss", "VnExpress", "khac", ""},
	{"vnexpress", "https://vnexpress.net/rss/thoi-su.rss", "VnExpress Thời sự", "thoi-su", ""},
	{"vnexpress", "https://vnexpress.net/rss/the-gioi.rss", "VnExpress Thế giới", "the-gioi", ""},
	{"vnexpress", "https://vnexpress.net/rss/kinh-doanh.rss", "VnExpress Kinh doanh", "kinh-doanh", ""},
	{"vnexpress", "https://vnexpress.net/rss/khoa-hoc-cong-nghe.rss", "VnExpress Khoa học công nghệ", "cong-nghe", ""},
	{"vnexpress", "https://vnexpress.net/rss/giai-tri.rss", "VnExpress Giải trí", "giai-tri", ""},
	{"vnexpress", "https://vnexpress.net/rss/the-thao.rss", "VnExpress Thể thao", "the-thao", ""},
	{"vnexpress", "https://vnexpress.net/rss/suc-khoe.rss", "VnExpress Sức khỏe", "suc-khoe", ""},
	{"vnexpress", "https://vnexpress.net/rss/gia-dinh.rss", "VnExpress Gia đình", "doi-song", ""},
	{"bbc", "https://feeds.bbci.co.uk/news/rss.xml", "BBC News", "khac", "/news"},
	{"bbc", "https://feeds.bbci.co.uk/news/world/rss.xml", "BBC World", "the-gioi", "/news/world"},
	{"bbc", "https://feeds.bbci.co.uk/news/business/rss.xml", "BBC Business", "kinh-doanh", "/news/business"},
	{"bbc", "https://feeds.bbci.co.uk/news/technology/rss.xml", "BBC Technology", "cong-nghe", "/news/technology"},
	{"bbc", "https://feeds.bbci.co.uk/news/entertainment_and_arts/rss.xml", "BBC Entertainment & Arts", "giai-tri", "/news/entertainment_and_arts"},
	{"bbc", "https://feeds.bbci.co.uk/news/health/rss.xml", "BBC Health", "suc-khoe", "/news/health"},
	{"tuoitre", "https://tuoitre.vn/thoi-su.rss", "Tuổi Trẻ Thời sự", "thoi-su", "/thoi-su.htm"},
	{"tuoitre", "https://tuoitre.vn/the-gioi.rss", "Tuổi Trẻ Thế giới", "the-gioi", "/the-gioi.htm"},
	{"tuoitre", "https://tuoitre.vn/kinh-doanh.rss", "Tuổi Trẻ Kinh doanh", "kinh-doanh", "/kinh-doanh.htm"},
	{"tuoitre", "https://tuoitre.vn/video.rss", "Tuổi Trẻ Video", "khac", ""},
}

var siteNames = map[string]string{"vnexpress": "VnExpress", "bbc": "BBC News", "tuoitre": "Tuổi Trẻ"}

// Import kinds.
const (
	ImportRSS        = "rss"
	ImportHome       = "home"
	ImportSection    = "section"
	ImportArticle    = "article"
	ImportUnknown    = "unsupported_page" // supported site, page type not recognised
	ImportUnsupport  = "unsupported_site"
	ImportInvalidURL = "invalid"
)

// Resolution is what an import URL was recognised as. It is computed from the
// URL alone: no request is made to recognise a URL.
type Resolution struct {
	Kind    string
	Adapter string
	Site    string
	Input   string        // the URL as typed (trimmed)
	Feeds   []CatalogFeed // feeds to add; Name/Category empty for an RSS URL outside the catalog
	Message string
}

// siteHosts: hosts accepted for each site. www.vnexpress.net (301) and
// www.tuoitre.vn (302) redirect to the bare host, bbc.com to www.bbc.com
// (checked 2026-10-05).
func siteOf(host string) string {
	switch host {
	case "vnexpress.net", "www.vnexpress.net":
		return "vnexpress"
	case "tuoitre.vn", "www.tuoitre.vn":
		return "tuoitre"
	case "www.bbc.co.uk", "www.bbc.com", "bbc.co.uk", "bbc.com", "feeds.bbci.co.uk":
		return "bbc"
	}
	return ""
}

var (
	vneArticlePath = regexp.MustCompile(`\.html$`)
	bbcArticlePath = regexp.MustCompile(`/(articles|videos|live|av)/|-\d{5,}$`)
	ttVideoPath    = regexp.MustCompile(`^/video/.+-\d+\.htm$`)
)

func catalogFor(adapter string) []CatalogFeed {
	out := []CatalogFeed{}
	for _, f := range Catalog {
		if f.Adapter == adapter {
			out = append(out, f)
		}
	}
	return out
}

func catalogFeed(feedURL string) (CatalogFeed, bool) {
	for _, f := range Catalog {
		if f.FeedURL == feedURL {
			return f, true
		}
	}
	return CatalogFeed{}, false
}

// ResolveImport recognises a URL typed into "Import nguồn".
func ResolveImport(raw string) Resolution {
	in := strings.TrimSpace(raw)
	r := Resolution{Input: in, Kind: ImportInvalidURL, Message: "URL không hợp lệ. Hãy dán URL đầy đủ, ví dụ https://vnexpress.net/rss/thoi-su.rss"}
	if in == "" || len(in) > 2000 || strings.ContainsAny(in, " \t\r\n") {
		return r
	}
	s := in
	if !strings.Contains(s, "://") {
		s = "https://" + s // "vnexpress.net/…" typed without a scheme
	}
	u, e := url.Parse(s)
	if e != nil || u.Host == "" {
		return r
	}
	if u.Scheme != "https" {
		r.Message = "Chỉ hỗ trợ URL https."
		return r
	}
	if u.User != nil {
		r.Message = "URL không được chứa tên đăng nhập hoặc mật khẩu."
		return r
	}
	if p := u.Port(); p != "" && p != "443" {
		r.Message = "URL không được chỉ định cổng khác 443."
		return r
	}
	host := strings.ToLower(u.Hostname())
	ad := siteOf(host)
	if ad == "" {
		r.Kind, r.Message = ImportUnsupport, "Nguồn này chưa được hỗ trợ. Hiện hỗ trợ VnExpress, BBC News và Tuổi Trẻ."
		return r
	}
	r.Adapter, r.Site = ad, siteNames[ad]
	path := u.EscapedPath()
	if path == "" {
		path = "/"
	}
	trimmed := strings.TrimSuffix(path, "/")
	if trimmed == "" {
		trimmed = "/"
	}
	feed := func(f string) Resolution {
		r.Kind = ImportRSS
		if c, ok := catalogFeed(f); ok {
			r.Feeds = []CatalogFeed{c}
		} else {
			r.Feeds = []CatalogFeed{{Adapter: ad, FeedURL: f}}
		}
		return r
	}
	home := func() Resolution {
		r.Kind, r.Feeds = ImportHome, catalogFor(ad)
		return r
	}
	unknown := func(msg string) Resolution {
		r.Kind, r.Message = ImportUnknown, msg
		return r
	}
	article := func() Resolution {
		r.Kind, r.Message = ImportArticle, "Đây là URL của một bài viết, chưa phải URL nguồn tin. Hãy dán URL trang chủ, trang chuyên mục hoặc RSS của "+r.Site+"."
		return r
	}
	switch ad {
	case "vnexpress":
		switch {
		case trimmed == "/" || trimmed == "/rss":
			return home()
		case strings.HasPrefix(trimmed, "/rss/") && strings.HasSuffix(trimmed, ".rss"):
			f := "https://vnexpress.net" + trimmed
			if !Allowed(f, ad, true) {
				return unknown("URL RSS VnExpress không hợp lệ.")
			}
			return feed(Canonical(f, ad, true))
		case vneArticlePath.MatchString(trimmed):
			return article()
		default:
			// VnExpress section pages do not declare their feed and the RSS
			// list does not link them, so a section URL is not mapped.
			return unknown("Chưa có ánh xạ đã xác minh từ trang chuyên mục VnExpress sang RSS. Hãy dán URL RSS của chuyên mục (danh sách tại https://vnexpress.net/rss).")
		}
	case "bbc":
		if host == "feeds.bbci.co.uk" {
			f := "https://feeds.bbci.co.uk" + trimmed
			if !strings.HasSuffix(trimmed, "/rss.xml") || !Allowed(f, ad, true) {
				return unknown("Chỉ hỗ trợ RSS BBC News dạng https://feeds.bbci.co.uk/news/…/rss.xml.")
			}
			return feed(f)
		}
		if trimmed == "/" {
			return home()
		}
		for _, c := range catalogFor(ad) {
			if c.Section == trimmed {
				r.Kind, r.Feeds = ImportSection, []CatalogFeed{c}
				return r
			}
		}
		if bbcArticlePath.MatchString(trimmed) {
			return article()
		}
		return unknown("Chưa có ánh xạ đã xác minh từ trang này của BBC sang RSS. Hãy dán URL RSS (https://feeds.bbci.co.uk/news/…/rss.xml).")
	case "tuoitre":
		switch {
		case trimmed == "/" || trimmed == "/rss.htm":
			return home()
		case strings.HasSuffix(trimmed, ".rss"):
			f := "https://tuoitre.vn" + strings.Replace(trimmed, "/rss/", "/", 1) // same feed, checked for thoi-su, the-gioi, kinh-doanh
			if !Allowed(f, ad, true) {
				return unknown("URL RSS Tuổi Trẻ không hợp lệ.")
			}
			return feed(f)
		case ArticleKey("https://tuoitre.vn"+trimmed, ad) != "" || ttVideoPath.MatchString(trimmed):
			return article()
		}
		for _, c := range catalogFor(ad) {
			if c.Section != "" && c.Section == trimmed {
				r.Kind, r.Feeds = ImportSection, []CatalogFeed{c}
				return r
			}
		}
		return unknown("Chưa có ánh xạ đã xác minh từ trang này của Tuổi Trẻ sang RSS. Hãy dán URL RSS (danh sách tại https://tuoitre.vn/rss.htm).")
	}
	return r
}

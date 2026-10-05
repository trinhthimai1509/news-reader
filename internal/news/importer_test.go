package news

import (
	"strings"
	"testing"
)

func TestResolveImport(t *testing.T) {
	type want struct {
		kind  string
		feeds int
		first string // first feed URL
	}
	cases := map[string]want{
		// Home pages: the checked catalog of the site.
		"https://vnexpress.net/":     {ImportHome, 9, "https://vnexpress.net/rss/tin-moi-nhat.rss"},
		"vnexpress.net":              {ImportHome, 9, ""},
		"https://www.vnexpress.net":  {ImportHome, 9, ""},
		"https://vnexpress.net/rss":  {ImportHome, 9, ""},
		"https://www.bbc.co.uk/":     {ImportHome, 6, "https://feeds.bbci.co.uk/news/rss.xml"},
		"https://tuoitre.vn/":        {ImportHome, 4, "https://tuoitre.vn/thoi-su.rss"},
		"https://tuoitre.vn/rss.htm": {ImportHome, 4, ""},
		"  https://tuoitre.vn  ":     {ImportHome, 4, ""},
		// RSS URLs.
		"https://vnexpress.net/rss/the-gioi.rss":                        {ImportRSS, 1, "https://vnexpress.net/rss/the-gioi.rss"},
		"https://vnexpress.net/rss/du-lich.rss?utm_source=x":            {ImportRSS, 1, "https://vnexpress.net/rss/du-lich.rss"},
		"https://feeds.bbci.co.uk/news/science_and_environment/rss.xml": {ImportRSS, 1, "https://feeds.bbci.co.uk/news/science_and_environment/rss.xml"},
		"https://tuoitre.vn/rss/thoi-su.rss":                            {ImportRSS, 1, "https://tuoitre.vn/thoi-su.rss"},
		"https://tuoitre.vn/giao-duc.rss":                               {ImportRSS, 1, "https://tuoitre.vn/giao-duc.rss"},
		// Section pages with a mapping taken from the feeds' channel links.
		"https://www.bbc.co.uk/news/world":                   {ImportSection, 1, "https://feeds.bbci.co.uk/news/world/rss.xml"},
		"https://www.bbc.com/news/business/":                 {ImportSection, 1, "https://feeds.bbci.co.uk/news/business/rss.xml"},
		"https://www.bbc.co.uk/news":                         {ImportSection, 1, "https://feeds.bbci.co.uk/news/rss.xml"},
		"https://tuoitre.vn/the-gioi.htm":                    {ImportSection, 1, "https://tuoitre.vn/the-gioi.rss"},
		"https://vnexpress.net/thoi-su":                      {ImportUnknown, 0, ""}, // no verified mapping
		"https://www.bbc.co.uk/news/science_and_environment": {ImportUnknown, 0, ""},
		"https://tuoitre.vn/giao-duc.htm":                    {ImportUnknown, 0, ""},
		// Article URLs are never feeds.
		"https://vnexpress.net/thu-tuong-nhat-diu-giong-5128557.html":                  {ImportArticle, 0, ""},
		"https://www.bbc.co.uk/news/articles/cme3r9ny2p4xo":                            {ImportArticle, 0, ""},
		"https://www.bbc.com/news/videos/c3eweld0nddeo":                                {ImportArticle, 0, ""},
		"https://tuoitre.vn/thu-tuong-tay-ban-nha-bat-ngo-100261005152148723.htm":      {ImportArticle, 0, ""},
		"https://tuoitre.vn/video/cong-an-tphcm-va-cuoc-truy-quet-hang-gia-203229.htm": {ImportArticle, 0, ""},
		// Unsupported sites.
		"https://nld.tuoitre.vn/":            {ImportUnsupport, 0, ""},
		"https://vnexpress.net.evil.com/rss": {ImportUnsupport, 0, ""},
		"https://example.com/feed.xml":       {ImportUnsupport, 0, ""},
		"https://thanhnien.vn/rss/home.rss":  {ImportUnsupport, 0, ""},
		// Invalid or unsafe URLs: rejected before any request.
		"http://vnexpress.net/rss/thoi-su.rss":       {ImportInvalidURL, 0, ""},
		"https://user:pass@vnexpress.net/":           {ImportInvalidURL, 0, ""},
		"https://vnexpress.net:8443/rss/thoi-su.rss": {ImportInvalidURL, 0, ""},
		"javascript:alert(1)":                        {ImportInvalidURL, 0, ""},
		"ftp://tuoitre.vn/":                          {ImportInvalidURL, 0, ""},
		"":                                           {ImportInvalidURL, 0, ""},
		"https://vnexpress.net/rss/a b.rss":          {ImportInvalidURL, 0, ""},
		"https://127.0.0.1/rss/thoi-su.rss":          {ImportUnsupport, 0, ""},
		"https://feeds.bbci.co.uk/sport/rss.xml":     {ImportUnknown, 0, ""},
	}
	for in, w := range cases {
		r := ResolveImport(in)
		if r.Kind != w.kind || len(r.Feeds) != w.feeds || (w.first != "" && r.Feeds[0].FeedURL != w.first) {
			t.Errorf("%q: got %s %d %+v (%s)", in, r.Kind, len(r.Feeds), r.Feeds, r.Message)
		}
		if w.feeds == 0 && r.Message == "" {
			t.Errorf("%q: no message", in)
		}
		for _, f := range r.Feeds {
			if !Allowed(f.FeedURL, f.Adapter, true) {
				t.Errorf("%q: feed outside the allowlist %s", in, f.FeedURL)
			}
		}
	}
	if r := ResolveImport("https://example.com/"); r.Message != "Nguồn này chưa được hỗ trợ. Hiện hỗ trợ VnExpress, BBC News và Tuổi Trẻ." {
		t.Fatal(r.Message)
	}
	// A catalog feed carries its checked name and category; others get them later.
	if r := ResolveImport("https://vnexpress.net/rss/gia-dinh.rss"); r.Feeds[0].Name != "VnExpress Gia đình" || r.Feeds[0].Category != "doi-song" {
		t.Fatalf("%+v", r.Feeds[0])
	}
	if r := ResolveImport("https://tuoitre.vn/giao-duc.rss"); r.Feeds[0].Name != "" || r.Feeds[0].Category != "" {
		t.Fatalf("%+v", r.Feeds[0])
	}
	if !strings.Contains(ResolveImport("https://vnexpress.net/a-5128557.html").Message, "bài viết") {
		t.Fatal("article message")
	}
}

func TestCatalogFeedsAreAllowed(t *testing.T) {
	seen := map[string]bool{}
	for _, f := range Catalog {
		if !Allowed(f.FeedURL, f.Adapter, true) || seen[f.FeedURL] || f.Name == "" || f.Category == "" {
			t.Errorf("%+v", f)
		}
		seen[f.FeedURL] = true
	}
}

func TestFeedTitle(t *testing.T) {
	if s := FeedTitle([]byte(`<rss><channel><title><![CDATA[Giáo dục - <b>RSS</b>]]></title></channel></rss>`), "tuoitre"); s != "Giáo dục - RSS" {
		t.Fatal(s)
	}
}

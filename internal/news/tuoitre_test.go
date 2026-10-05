package news

import (
	"strings"
	"testing"
	"time"
)

const ttPage = "https://tuoitre.vn/bai-viet-mau-100261005145713396.htm"

const ttLong = "Đoạn văn mẫu đủ dài để giống một đoạn của bài báo thật, có nhiều chữ và dấu câu đầy đủ như trên trang nguồn. Câu thứ hai của đoạn văn mẫu cũng dài tương tự. "

// ttDoc mirrors the structure seen on tuoitre.vn article pages (2026-10-05).
func ttDoc(head, author, body string) []byte {
	return []byte(`<html><head><meta name="author" content="TUOI TRE ONLINE"><meta property="og:type" content="article">` + head + `</head><body>
<div class="detail-top"><div class="detail-time"><time data-role="publishdate">05/10/2026 15:42 GMT+7</time></div></div>
<h1 class="detail-title" data-role="title">Tiêu đề</h1>
` + author + `
<p class="detail-sapo" data-role="sapo">Sapo của bài.</p>
<div class="detail-cmain"><div class="detail-content afcbc-body" data-role="content" itemprop="articleBody">` + body + `</div>
<div class="box-author-detail"><div class="detail-author-bot" data-role="author"><a class="name" title="BOT">BOT</a></div></div></div>
<div class="main-content-body"><div id="objectPopupBody" class="content fck"></div></div>
<section class="comment-wrapper"><p>Bình luận của bạn đọc không phải nội dung bài.</p></section>
</body></html>`)
}

func ttAuthor(names ...string) string {
	s := `<div class="detail-author oneauthor" data-role="author"><div class="groupavtauthor"><a class="avata" title="` + names[0] + `"><img class="avt isauthor" src="" alt="tác giả"></a></div>
<div class="author-info"><a class="name" title="` + names[0] + `">` + names[0] + `</a><span class="morenameauthor">và 2 tác giả khác</span></div><div class="swiper"><div class="swiper-wrapper">`
	for _, n := range names {
		s += `<div class="swiper-slide"><div class="author-item"><div class="author-item-avatar"><a title="` + n + `"><img class="isauthor" src="" alt="` + n + `"></a></div><div class="author-item-name"><a class="name" title="` + n + `">` + n + `</a></div></div></div>`
	}
	return s + `</div></div></div>`
}

const ttPhoto = `<figure class="VCSortableInPreviewMode" type="Photo"><div><img src="https://cdn2.tuoitre.vn/thumb_w/730/471584752817336320/2026/10/5/a-1.jpg" w="2560" h="1707" alt="Tiêu đề - Ảnh 1." data-original="https://cdn2.tuoitre.vn/471584752817336320/2026/10/5/a-1.jpg" type="photo" width="2560" height="1707"></div><figcaption class="PhotoCMS_Caption"><p>Thủ tướng phát biểu tại Quốc hội - Ảnh: AFP</p></figcaption></figure>`

const ttRelated = `<div class="VCSortableInPreviewMode alignRight" type="RelatedNewsBox"><ul><li><a href="https://tuoitre.vn/khac-1.htm"><img src="https://cdn2.tuoitre.vn/thumb_w/730/rel.jpg" alt="liên quan"></a><h4><a>Bài liên quan</a></h4><a><span>ĐỌC NGAY</span></a></li></ul></div>
<div type="RelatedOneNews" class="VCSortableInPreviewMode"><a href="https://tuoitre.vn/khac-2.htm"><img src="https://cdn2.tuoitre.vn/thumb_w/730/rel2.jpg"></a><a class="OneNewsTitle">Bài khác</a><p class="VCObjectBoxRelatedNewsItemSapo">Sapo bài khác.</p></div>`

func TestTuoiTreArticle(t *testing.T) {
	body := ttPhoto + `<p>` + ttLong + `<a class="seo-suggest-link" href="https://tuoitre.vn/x.html">Ngoại thương</a>, tiếp tục.</p><h2>Tiêu đề phụ</h2>` + ttRelated +
		`<p>` + ttLong + `</p><div class="VCSortableInPreviewMode alignCenter" type="content"><div><p>Đoạn trong hộp nội dung.</p></div></div>
<div type="VideoStream" class="VCSortableInPreviewMode"><video src="https://x/v.mp4"></video><p>Mô tả video</p></div>
<figure type="Photo"><img src="https://static-tuoitre.tuoitre.vn/logo.png" w="100" h="100"></figure>
<div class="readmore-body-box d-none"><a>Đọc tiếp</a></div>`
	c, e := ExtractArticle(ttDoc(`<meta property="og:image" content="https://cdn2.tuoitre.vn/zoom/1200_630/og.jpg">`, ttAuthor("KỲ PHONG"), body), "tuoitre", ttPage)
	if e != nil {
		t.Fatal(e)
	}
	if strings.Join(c.Authors, "|") != "KỲ PHONG" {
		t.Fatalf("authors %q", c.Authors)
	}
	kinds := []string{}
	for _, b := range c.Blocks {
		kinds = append(kinds, b.Type)
	}
	if strings.Join(kinds, ",") != "img,p,p,p,p,video" || c.Blocks[5].Video.Kind != "link" {
		t.Fatalf("order %v: %+v", kinds, c.Blocks)
	}
	im := c.Blocks[0].Image
	if im.Src != "https://cdn2.tuoitre.vn/thumb_w/730/471584752817336320/2026/10/5/a-1.jpg" || im.Width != 2560 || im.Height != 1707 ||
		im.Caption != "Thủ tướng phát biểu tại Quốc hội" || im.Credit != "AFP" || !strings.Contains(im.Srcset, "a-1.jpg 2560w") {
		t.Fatalf("image %+v", im)
	}
	all := strings.Join(c.Paragraphs(), "\n")
	for _, bad := range []string{"Bài liên quan", "ĐỌC NGAY", "Sapo bài khác", "Mô tả video", "Đọc tiếp", "Bình luận", "BOT", "TUOI TRE", "Ảnh: AFP", "Sapo của bài"} {
		if strings.Contains(all, bad) {
			t.Errorf("%q leaked into body", bad)
		}
	}
	if !strings.Contains(all, "Ngoại thương, tiếp tục.") || !strings.Contains(all, "Tiêu đề phụ") || !strings.Contains(all, "Đoạn trong hộp nội dung.") {
		t.Fatalf("body: %s", all)
	}
	if c.Lead != nil {
		t.Fatal("og:image is a separate crop; not shown when the body has an image")
	}
}

func TestTuoiTreAuthors(t *testing.T) {
	cases := map[string][]string{
		ttAuthor("ĐAN THUẦN", "CHÍ KIÊN", "MEDIA"): {"ĐAN THUẦN", "CHÍ KIÊN"},
		ttAuthor("TUỔI TRẺ ONLINE"):                nil,
		ttAuthor("TS TRẦN HỮU HIỆP"):               {"TS TRẦN HỮU HIỆP"},
		``:                                         nil, // no byline: only meta author "TUOI TRE ONLINE"
	}
	for in, want := range cases {
		c, _ := ExtractArticle(ttDoc("", in, `<p>`+ttLong+`</p><p>`+ttLong+`</p><p>`+ttLong+`</p>`), "tuoitre", ttPage)
		if strings.Join(c.Authors, "|") != strings.Join(want, "|") {
			t.Errorf("got %q want %q", c.Authors, want)
		}
	}
}

func TestTuoiTreNotFull(t *testing.T) {
	// /video/ page: og:type website, no paragraphs.
	c, e := ExtractArticle(ttDoc(`<meta property="og:type" content="website">`, ttAuthor("ĐAN THUẦN"), ttPhoto), "tuoitre", "https://tuoitre.vn/video/mau-203229.htm")
	if e == nil || len(c.Blocks) != 0 || c.Authors[0] != "ĐAN THUẦN" {
		t.Fatalf("video page: %v %+v", e, c)
	}
	// Teaser only.
	if _, e = ExtractArticle(ttDoc("", "", `<p>Ngắn.</p>`), "tuoitre", ttPage); e == nil {
		t.Fatal("short page marked full")
	}
	// Unknown layout: no data-role=content body.
	if _, e = ExtractArticle([]byte(`<html><body><div class="content fck"><p>`+ttLong+`</p><p>`+ttLong+`</p><p>`+ttLong+`</p></div></body></html>`), "tuoitre", ttPage); e == nil {
		t.Fatal("unknown layout marked full")
	}
	for _, in := range []string{``, `<div data-role="content" class="detail-content"><figure type="Photo"></figure><figure type="Photo"><img></figure></div><div class="detail-author" data-role="author"><div class="author-item-name"></div></div>`} {
		ExtractArticle([]byte(in), "tuoitre", ttPage) // must return, not panic
	}
}

func TestTuoiTreSources(t *testing.T) {
	for _, u := range []string{"https://tuoitre.vn/thoi-su.rss", "https://tuoitre.vn/rss/the-gioi.rss"} {
		if !Allowed(u, "tuoitre", true) {
			t.Error(u)
		}
	}
	for _, u := range []string{"https://tuoitre.vn/thoi-su.htm", "https://nld.tuoitre.vn/thoi-su.rss", "https://tuoitre.vn/nld/a.rss", "http://tuoitre.vn/thoi-su.rss", "https://vnexpress.net/rss/thoi-su.rss", "https://congnghe.tuoitre.vn/a.rss"} {
		if Allowed(u, "tuoitre", true) {
			t.Error("feed allowed:", u)
		}
	}
	if !Allowed(ttPage, "tuoitre", false) || Allowed("https://tuoitre.vn/nld/a-1.htm", "tuoitre", false) || Allowed("https://cdn2.tuoitre.vn/a.htm", "tuoitre", false) || Allowed("https://tuoitre.vn/a.rss", "tuoitre", false) {
		t.Error("article allowlist")
	}
	if Allowed("https://tuoitre.vn/thoi-su.rss", "vnexpress", true) {
		t.Error("tuoitre feed accepted for another adapter")
	}
}

func TestTuoiTreArticleKey(t *testing.T) {
	// Verified 2026-10-05: another slug with this id answers 301 to the current one.
	a := ArticleKey("https://tuoitre.vn/thu-tuong-tay-ban-nha-bat-ngo-keu-goi-bau-cu-som-100261005152148723.htm", "tuoitre")
	b := ArticleKey("https://tuoitre.vn/abc-100261005152148723.htm", "tuoitre")
	if a != "tuoitre:100261005152148723" || a != b {
		t.Fatalf("%q %q", a, b)
	}
	if k := ArticleKey("https://tuoitre.vn/khach-mice-nam-2027-duoc-san-don-tu-som-10026100513194289.htm", "tuoitre"); k != "tuoitre:10026100513194289" {
		t.Fatal(k)
	}
	for _, u := range []string{"https://tuoitre.vn/video/cong-an-tphcm-203229.htm", "https://tuoitre.vn/thoi-su.htm", "https://tuoitre.vn/a-100261005152148723.htm?x=1", "https://tuoitre.vn/a/b-100261005152148723.htm"} {
		if k := ArticleKey(u, "tuoitre"); k != "" {
			t.Errorf("%s: %q", u, k)
		}
	}
	// The Tuổi Trẻ shape is not applied to other adapters.
	if k := ArticleKey("https://vnexpress.net/a-100261005152148723.htm", "vnexpress"); k != "" {
		t.Fatal(k)
	}
}

func TestTuoiTreDates(t *testing.T) {
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	got := PublishedIn("10/5/2026 3:42:00 PM", "tuoitre", now)
	if !got.Equal(time.Date(2026, 10, 5, 8, 42, 0, 0, time.UTC)) {
		t.Fatal(got)
	}
	if got := PublishedIn("10/4/2026 9:05:00 AM", "tuoitre", now); !got.Equal(time.Date(2026, 10, 4, 2, 5, 0, 0, time.UTC)) {
		t.Fatal(got)
	}
	if got := PublishedIn("10/6/2026 3:42:00 PM", "tuoitre", now); !got.Equal(now) {
		t.Fatal("future date not clamped", got)
	}
	// Other adapters keep RFC 1123 parsing; the US-style format is not guessed for them.
	if got := PublishedIn("10/5/2026 3:42:00 PM", "vnexpress", now); !got.Equal(now) {
		t.Fatal(got)
	}
}

func TestTuoiTreFeedImage(t *testing.T) {
	it := Item{}
	it.Enclosure.URL, it.Enclosure.Type = "https://cdn2.tuoitre.vn/thumb_w/1200/a.jpg", "image/jpeg"
	if im := it.FeedImage("tuoitre"); im == nil || im.Src != it.Enclosure.URL {
		t.Fatal(im)
	}
	it.Enclosure.URL = "https://static-tuoitre.tuoitre.vn/a.jpg" // not a verified body-image host
	if im := it.FeedImage("tuoitre"); im != nil {
		t.Fatal(im)
	}
}

func TestTuoiTreDecomposedTextIsComposed(t *testing.T) {
	nfd := "THÁI BÁ DŨNG" // as served in a real byline
	c, e := ExtractArticle(ttDoc("", ttAuthor(nfd), `<p>`+ttLong+`</p><p>Các đoạn `+ttLong+`</p>`), "tuoitre", ttPage)
	if e != nil || len(c.Authors) != 1 || c.Authors[0] != "THÁI BÁ DŨNG" || !strings.HasPrefix(c.Blocks[1].Text, "Các đoạn") {
		t.Fatalf("%v %q %+v", e, c.Authors, c.Blocks)
	}
	if PlainIn("Thủ tướng", "tuoitre") != "Thủ tướng" {
		t.Fatal("feed title not composed")
	}
}

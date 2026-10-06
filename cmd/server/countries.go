package main

import (
	"encoding/json"
	"net/http"
	"regexp"
)

// The country of an article is the country of its publisher (the website
// that published it), set by the administrator. It never comes from the
// article text, the domain or the title. NULL = not determined.

var countryPattern = regexp.MustCompile(`^[A-Z]{2}$`)

// countryUnknown selects articles whose publisher has no country.
const countryUnknown = "unknown"

func validCountryFilter(v string) bool {
	return v == "" || v == countryUnknown || countryPattern.MatchString(v)
}

// countryMatches filters on publisher p (joined on the article's source).
const countryMatches = `(%[1]s='' OR (%[1]s='unknown' AND p.country IS NULL) OR p.country=%[1]s)`

type Country struct {
	Code  string `json:"code"`
	Name  string `json:"name"`
	InUse bool   `json:"in_use"` // a non-deleted source's publisher has it
}

// handleCountries (public) lists the countries that can be chosen; in_use
// marks those the reader filter offers. unknown_in_use: some source's
// publisher has no country yet ("Chưa xác định").
func (a *App) handleCountries(w http.ResponseWriter, r *http.Request) {
	rows, e := a.db.Query(r.Context(), `SELECT c.code,c.name,EXISTS(SELECT 1 FROM sources s JOIN publishers p ON p.adapter=s.adapter WHERE NOT s.deleted AND p.country=c.code)
FROM countries c ORDER BY c.position,c.code`)
	if e != nil {
		fail(w, 500, "Không đọc được danh sách quốc gia")
		return
	}
	defer rows.Close()
	out := []Country{}
	for rows.Next() {
		var c Country
		if rows.Scan(&c.Code, &c.Name, &c.InUse) != nil {
			fail(w, 500, "Không đọc được danh sách quốc gia")
			return
		}
		out = append(out, c)
	}
	if rows.Err() != nil {
		fail(w, 500, "Không đọc được danh sách quốc gia")
		return
	}
	var unknown bool
	if a.db.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM sources s JOIN publishers p ON p.adapter=s.adapter WHERE NOT s.deleted AND p.country IS NULL)`).Scan(&unknown) != nil {
		fail(w, 500, "Không đọc được danh sách quốc gia")
		return
	}
	send(w, 200, map[string]any{"countries": out, "unknown_in_use": unknown})
}

type Publisher struct {
	Adapter string `json:"adapter"`
	Name    string `json:"name"`
	Country string `json:"country"` // "" = Chưa xác định
	Feeds   int    `json:"feeds"`   // non-deleted feeds of this website
}

func (a *App) handlePublishers(w http.ResponseWriter, r *http.Request) {
	rows, e := a.db.Query(r.Context(), `SELECT p.adapter,p.name,coalesce(p.country,''),(SELECT count(*) FROM sources s WHERE s.adapter=p.adapter AND NOT s.deleted) FROM publishers p ORDER BY p.name`)
	if e != nil {
		fail(w, 500, "Không đọc được danh sách website")
		return
	}
	defer rows.Close()
	out := []Publisher{}
	for rows.Next() {
		var p Publisher
		if rows.Scan(&p.Adapter, &p.Name, &p.Country, &p.Feeds) != nil {
			fail(w, 500, "Không đọc được danh sách website")
			return
		}
		out = append(out, p)
	}
	if rows.Err() != nil {
		fail(w, 500, "Không đọc được danh sách website")
		return
	}
	send(w, 200, out)
}

// handleSetPublisherCountry sets the country of one website, and so of all
// its feeds. {"country": null} sets it back to "Chưa xác định".
func (a *App) handleSetPublisherCountry(w http.ResponseWriter, r *http.Request) {
	// A map keeps an explicit null apart from a missing key.
	var b map[string]json.RawMessage
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	var raw json.RawMessage
	ok := false
	if json.NewDecoder(r.Body).Decode(&b) == nil {
		raw, ok = b["country"]
	}
	if !ok {
		fail(w, 400, "Dữ liệu không hợp lệ")
		return
	}
	var country *string
	if string(raw) != "null" {
		var c string
		if json.Unmarshal(raw, &c) != nil || !countryPattern.MatchString(c) {
			fail(w, 400, "Mã quốc gia không hợp lệ")
			return
		}
		country = &c
	}
	tag, e := a.db.Exec(r.Context(), `UPDATE publishers SET country=$2 WHERE adapter=$1`, r.PathValue("adapter"), country)
	if isUnknownCategory(e) { // foreign key: not in countries
		fail(w, 400, "Quốc gia không có trong danh sách")
		return
	}
	if e != nil {
		fail(w, 500, "Không cập nhật được quốc gia")
		return
	}
	if tag.RowsAffected() == 0 {
		fail(w, 404, "Không tìm thấy website")
		return
	}
	send(w, 200, map[string]bool{"ok": true})
}

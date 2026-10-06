// Reader filters: pure helpers, used by app.js and tested with node --test.
// The country is the country of the news publisher (set by the
// administrator for each website), never the country an article is about.
(function (root) {
  const UNKNOWN = 'unknown';
  const UNKNOWN_LABEL = 'Chưa xác định';

  // Query string for GET /api/articles. Empty filters are still sent so the
  // server sees exactly what the page shows.
  function listParams({ page, category, source, country, query }) {
    return new URLSearchParams({ page: String(page || 1), source: source || '', q: query || '', category: category || '', country: country || '' });
  }

  // Options of the reader's country filter: countries that some source has,
  // then "Chưa xác định" when a source has none.
  function countryOptions(res) {
    const out = [{ value: '', label: 'Tất cả quốc gia' }];
    for (const c of (res && res.countries) || []) if (c.in_use) out.push({ value: c.code, label: c.name });
    if (res && res.unknown_in_use) out.push({ value: UNKNOWN, label: UNKNOWN_LABEL });
    return out;
  }

  function countryName(code, countries) {
    if (!code) return UNKNOWN_LABEL;
    const c = (countries || []).find(x => x.code === code);
    return c ? c.name : code;
  }

  // Sources offered by the source filter for the chosen country.
  function sourcesForCountry(sources, country) {
    if (!country) return sources;
    return sources.filter(s => (country === UNKNOWN ? !s.country : s.country === country));
  }

  // After the country changes, keep the chosen source only if it belongs to
  // that country; otherwise fall back to all sources.
  function keepSource(sources, country, source) {
    return source && sourcesForCountry(sources, country).some(s => String(s.id) === String(source)) ? source : '';
  }

  const api = { listParams, countryOptions, countryName, sourcesForCountry, keepSource, UNKNOWN, UNKNOWN_LABEL };
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
  else root.Filters = api;
})(typeof window !== 'undefined' ? window : globalThis);

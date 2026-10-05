// "Import nguồn": pure helpers, used by app.js and tested with node --test.
(function (root) {
  const REPORT_TO = 'trinhthimai1509@gmail.com';
  const REPORT_SUBJECT = 'Lỗi import';

  // mailto with only the URL the user typed (trimmed) as the body: no token,
  // log or machine details. Opened only when the user clicks the button.
  function reportHref(input) {
    return `mailto:${REPORT_TO}?subject=${encodeURIComponent(REPORT_SUBJECT)}&body=${encodeURIComponent(String(input).trim())}`;
  }

  // What to show for a server answer. report: offer "Báo lỗi qua email".
  // Authentication errors are not source errors (the app asks to log in).
  function outcome(status, body) {
    if (status === 401) return { tone: 'auth', report: false, text: 'Phiên đã hết hoặc cần mã truy cập. Hãy đăng nhập lại rồi import.' };
    if (status === 429) return { tone: 'error', report: false, text: (body && body.error) || 'Đang có lượt import khác hoặc import quá nhiều lần; thử lại sau.' };
    if (!body || typeof body !== 'object') return { tone: 'error', report: true, text: `Máy chủ trả lỗi HTTP ${status}.` };
    if (body.error) return { tone: 'error', report: true, text: body.error };
    const lines = [];
    const list = (title, items) => { if (items && items.length) lines.push(`${title}: ${items.map(f => f.name + (f.note ? ` (${f.note})` : '') + (f.error ? ` — ${f.error}` : '')).join('; ')}`); };
    list('Đã thêm', body.added); list('Đã khôi phục', body.restored); list('Đã có', body.existing); list('Lỗi', body.failed);
    const tone = { ok: 'ok', exists: 'ok', partial: 'warn', failed: 'error', rejected: 'error' }[body.status] || 'error';
    // An article URL or an unsupported page is the user's input, but they may
    // still want to report it; a source that already exists is not an error.
    const report = body.status === 'partial' || body.status === 'failed' || body.status === 'rejected';
    return { tone, report, text: body.message || '', details: lines };
  }

  const api = { reportHref, outcome, REPORT_TO, REPORT_SUBJECT };
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
  else root.Importer = api;
})(typeof window !== 'undefined' ? window : globalThis);

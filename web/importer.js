// "Import nguồn": pure helpers, used by app.js and tested with node --test.
(function (root) {
  const REPORT_SUBJECT = 'Lỗi import';

  // mailto with only the URL the user typed (trimmed) as the body: no
  // cookie, log or machine details. Opened only when the user clicks the
  // button. The address comes from the admin session (REPORT_EMAIL), so it
  // is not in the public page.
  function reportHref(input, to) {
    return `mailto:${to}?subject=${encodeURIComponent(REPORT_SUBJECT)}&body=${encodeURIComponent(String(input).trim())}`;
  }

  // What to show for a server answer. report: offer "Báo lỗi qua email".
  // Authentication errors are not source errors (the app asks to log in).
  function outcome(status, body) {
    if (status === 401) return { tone: 'auth', report: false, text: 'Phiên đăng nhập đã hết. Hãy đăng nhập quản trị lại rồi import.' };
    if (status === 403) return { tone: 'auth', report: false, text: (body && body.error) || 'Yêu cầu không hợp lệ, hãy tải lại trang.' };
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

  const api = { reportHref, outcome, REPORT_SUBJECT };
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
  else root.Importer = api;
})(typeof window !== 'undefined' ? window : globalThis);

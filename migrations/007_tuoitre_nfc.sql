-- Tuổi Trẻ text stored before NFC normalisation (some bylines and summaries
-- came decomposed). Only rows that are not already NFC change; safe to rerun.
UPDATE articles a SET title=normalize(title,NFC), summary=normalize(summary,NFC),
 authors=ARRAY(SELECT normalize(x,NFC) FROM unnest(authors) WITH ORDINALITY u(x,i) ORDER BY i)
FROM sources s WHERE s.id=a.source_id AND s.adapter='tuoitre'
 AND (title IS NOT NFC NORMALIZED OR summary IS NOT NFC NORMALIZED OR array_to_string(authors,'|') IS NOT NFC NORMALIZED);

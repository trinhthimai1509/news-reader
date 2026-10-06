-- Country of the news publisher (the website), not of the places an article
-- mentions. It is set on the publisher, so every feed of one website has the
-- same country. NULL = not determined ("Chưa xác định"); nothing is guessed
-- from domains or titles. Every statement is additive and idempotent.
CREATE TABLE IF NOT EXISTS countries (
 code TEXT PRIMARY KEY CHECK (code ~ '^[A-Z]{2}$'), -- ISO 3166-1 alpha-2
 name TEXT NOT NULL,
 position INTEGER NOT NULL
);
-- Choices offered when managing sources. Adding a country here creates no
-- source and no article.
INSERT INTO countries(code,name,position) VALUES
 ('VN','Việt Nam',1),('GB','Vương quốc Anh',2),('US','Hoa Kỳ',3),('TH','Thái Lan',4),('CN','Trung Quốc',5),
 ('JP','Nhật Bản',6),('KR','Hàn Quốc',7),('SG','Singapore',8),('MY','Malaysia',9),('ID','Indonesia',10),
 ('PH','Philippines',11),('KH','Campuchia',12),('LA','Lào',13),('MM','Myanmar',14),('BN','Brunei',15),
 ('TL','Đông Timor',16),('TW','Đài Loan',17),('HK','Hồng Kông',18),('MO','Ma Cao',19),('MN','Mông Cổ',20),
 ('KP','Triều Tiên',21),('IN','Ấn Độ',22),('PK','Pakistan',23),('BD','Bangladesh',24),('LK','Sri Lanka',25),
 ('NP','Nepal',26),('AU','Úc',27),('NZ','New Zealand',28),('CA','Canada',29),('MX','Mexico',30),
 ('BR','Brazil',31),('AR','Argentina',32),('CL','Chile',33),('CO','Colombia',34),('PE','Peru',35),
 ('FR','Pháp',36),('DE','Đức',37),('IT','Ý',38),('ES','Tây Ban Nha',39),('PT','Bồ Đào Nha',40),
 ('NL','Hà Lan',41),('BE','Bỉ',42),('CH','Thụy Sĩ',43),('AT','Áo',44),('IE','Ireland',45),
 ('SE','Thụy Điển',46),('NO','Na Uy',47),('DK','Đan Mạch',48),('FI','Phần Lan',49),('PL','Ba Lan',50),
 ('CZ','Séc',51),('HU','Hungary',52),('RO','Romania',53),('GR','Hy Lạp',54),('UA','Ukraine',55),
 ('RU','Nga',56),('TR','Thổ Nhĩ Kỳ',57),('IL','Israel',58),('SA','Ả Rập Xê Út',59),('AE','Các Tiểu vương quốc Ả Rập Thống nhất',60),
 ('QA','Qatar',61),('IR','Iran',62),('EG','Ai Cập',63),('ZA','Nam Phi',64),('NG','Nigeria',65),
 ('KE','Kenya',66)
 ON CONFLICT(code) DO NOTHING;

-- One row per website. The adapter already identifies the website (every
-- feed of vnexpress.net uses adapter 'vnexpress'), so it is the key.
CREATE TABLE IF NOT EXISTS publishers (
 adapter TEXT PRIMARY KEY,
 name TEXT NOT NULL,
 country TEXT REFERENCES countries(code)
);
INSERT INTO publishers(adapter,name,country) VALUES
 ('vnexpress','VnExpress','VN'),
 ('bbc','BBC News','GB'),
 ('tuoitre','Tuổi Trẻ Online','VN')
 ON CONFLICT(adapter) DO NOTHING;
-- Any other adapter already in use gets a row with an undetermined country.
INSERT INTO publishers(adapter,name) SELECT DISTINCT adapter,adapter FROM sources ON CONFLICT(adapter) DO NOTHING;

DO $$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='sources_publisher_fk') THEN
  ALTER TABLE sources ADD CONSTRAINT sources_publisher_fk FOREIGN KEY (adapter) REFERENCES publishers(adapter);
 END IF;
END $$;

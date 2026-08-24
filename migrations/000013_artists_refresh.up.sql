-- Sync artists list with the updated roster (missing entries + case fix).
INSERT INTO gemfactory.artists (name, gender, is_active) VALUES
('ALICE SYNDROME', 'female', true), ('AWU', 'female', true), ('DIA', 'female', true), ('EVERGLOW', 'female', true),
('HEART OF WOMAN', 'female', true), ('I.O.I', 'female', true), ('Keyveatz', 'female', true), ('LEE CHAEYEON', 'female', true),
('LEE YOUNGJI', 'female', true), ('MIMI', 'female', true), ('OURBIRTHDAY', 'female', true), ('UNCHILD', 'female', true),
('BIGBANG', 'male', true), ('EXO', 'male', true), ('J.Y. Park', 'male', true), ('LNGSHOT', 'male', true),
('NCT 127', 'male', true), ('Picheolin', 'male', true), ('TOMORROW X TOGETHER', 'male', true), ('XLOV', 'male', true),
('YEONJUN', 'male', true)
ON CONFLICT (name) DO UPDATE SET gender = EXCLUDED.gender, is_active = true;

-- Fix lowercase duplicate of TUIDE, keeping its artist_id for existing releases.
UPDATE gemfactory.artists SET name = 'TUIDE' WHERE name = 'tuide';

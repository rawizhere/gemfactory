-- Preferred source languages when translating subtitles into ru.
INSERT INTO gemfactory.config (key, value, description) VALUES
('SUBS_SOURCE_PREF_RU', 'en,ko', 'Preferred source subtitle languages for RU translation (comma-separated, tried in order before the video language)')
ON CONFLICT (key) DO NOTHING;

-- Translation configuration and customizable prompt seed.
INSERT INTO gemfactory.config (key, value, description) VALUES
('TRANSLATION_PROVIDER', 'google', 'Translation provider (google, gemini, groq)'),
('GEMINI_API_KEY', '', 'Google AI Studio API Key for Gemini translations'),
('GROQ_API_KEY', '', 'Groq API Key for subtitle translations'),
('TRANSLATION_PROMPT', 'Translate for video subtitles (music videos, variety shows, livestreams):
1. Use natural, modern spoken language. Avoid wooden or robotic phrasing.
2. Proper nouns, stage/idol names, and fandom terms must stay as names (e.g. Winter, Joy, Karina — do not translate as dictionary words).
3. Keep speaker tags like [ALL], [WONYOUNG], (Chorus) untouched.
4. For song lyrics: translate the meaning and vibe poetically, not word-by-word.', 'Custom prompt rules for AI subtitle translation'),
('DOWNLOAD_CONCURRENCY', '4', 'Max concurrent video downloads and re-encodes')
ON CONFLICT (key) DO NOTHING;

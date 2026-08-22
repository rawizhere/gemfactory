-- Translation configuration and customizable prompt seed.
INSERT INTO gemfactory.config (key, value, description) VALUES
('TRANSLATION_PROVIDER', 'google', 'Translation provider (google, gemini, groq)'),
('GEMINI_API_KEY', '', 'Google AI Studio API Key for Gemini translations'),
('GROQ_API_KEY', '', 'Groq API Key for subtitle translations'),
('TRANSLATION_PROMPT', 'RULES ON NAMES & PROPER NOUNS:
1. NEVER translate proper names, stage names, idol/artist names, group names, or personal names into dictionary words/common nouns (e.g., ''Winter'', ''Joy'', ''Solar'', ''Rain'', ''Summer'' must remain names and not be translated into common seasonal/weather nouns). Keep names in their original Latin form or as proper name transliterations.
2. NEVER translate or delete speaker/singer tags or bracketed identifiers (e.g. [SUI], [WONYOUNG], [ALL], (Chorus), [수이], SUI:) — keep them verbatim at the start of each line.
3. Preserve emotional tone, slang, honorifics (like Unnie/Oppa/Hyung if appropriate) and conversational/lyric style.', 'Custom prompt rules for AI subtitle translation'),
('DOWNLOAD_CONCURRENCY', '4', 'Max concurrent video downloads and re-encodes')
ON CONFLICT (key) DO NOTHING;

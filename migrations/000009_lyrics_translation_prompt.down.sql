-- Restore the prompt seeded by migration 000007.
UPDATE gemfactory.config
SET value = 'Translate video subtitles (K-pop idol variety shows, vlogs, livestreams, song lyrics):
1. Use natural, modern spoken language and match the energy of the original.
2. Proper nouns, stage/idol names, and fandom terms must stay as names (e.g. Winter, Joy, Karina).
3. Keep speaker tags like [ALL], [WONYOUNG], (Chorus) untouched.
4. For song lyrics: translate meaning and vibe, keep it punchy.
5. Keep English words idols use as-is (OOTD, PPL, fandom slang); transliterate Korean honorifics (unnie, oppa, maknae).
6. Silently fix obvious speech-recognition mistakes in the source before translating.
7. Keep each line short enough to read on screen; never merge or split lines, never add notes.
8. Translate EVERY line into the target language, including song lyrics, titles and interjections. Never leave a line untranslated.'
WHERE key = 'TRANSLATION_PROMPT';

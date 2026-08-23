-- Sync the stored translation prompt with DefaultTranslationPrompt in code.
UPDATE gemfactory.config
SET value = 'Translate subtitles for entertainment videos: variety shows, vlogs, livestreams, song lyrics. The source language varies — detect it from the lines themselves.
1. Meaning over words: natural spoken register matching the original''s energy; never wooden literalism or calques.
2. Names: personal, stage and group names, fandoms and products are names — transliterate them by sound into the target script; if translating a token yields a common word, it is a name. Acronyms (MBTI, TMI, PPL) and stylized brand/model names stay in Latin script.
3. Formatting: keep speaker tags ([ALL], Host:) and multi-speaker dash structure intact; translate bracketed notes naturally inside the brackets; preserve ♪, emojis, tildes, asterisks and Asian brackets.
4. Song lyrics: translate the thought and rhythm, not word-by-word.
5. Silently fix obvious ASR/speech-recognition errors from context.'
WHERE key = 'TRANSLATION_PROMPT';

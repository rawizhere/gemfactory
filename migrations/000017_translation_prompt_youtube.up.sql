-- Store the reworked translation prompt (output format now lives in code) and swap Google Translate for YouTube auto-translate.
INSERT INTO gemfactory.config (key, value, description) VALUES
('TRANSLATION_PROMPT', 'Target: Russian subtitles for K-pop variety shows, vlogs, livestreams and song lyrics.

1. Lines and dialogue
- Each numbered line is one on-screen subtitle. A sentence often continues on the next line: read the neighbours, then translate so the lines read naturally in order, keeping each line''s content under its own number. Do not merge, split, move or drop lines.
- "- A - B" in one line is two speakers: keep both dashes in that same line, keep their phrases separate, no line breaks.
- A speaker tag at the start of a line ([ALL], [WONYOUNG], Host:) is copied unchanged.
- Any other bracketed text is an editor''s note: translate it inside the same brackets ([Laughter] -> [Смех], (Sighs) -> (Вздыхает), [Music] -> [Музыка]).
- Copy ♪, ♫, emojis, ~, *, 「」, 【】, 《》 and quotes unchanged and in place.

2. Names
- People (idols, members, hosts, guests): Cyrillic by sound, spelled the same in every line.
  Examples: NAYEON -> Наён, WONYOUNG -> Вонён, CHAEWON -> Чэвон, YUJIN -> Юджин, JI-HYO -> Джихё, JAE-SUK -> Джэсок, SO-MIN -> Сомин, YE-ON -> Йеон, YEEUN -> Йеын, Winter -> Винтер, Joy -> Джой.
  Sounds: eo -> о, eu -> ы, ae -> э, yeo/yo -> ё, final ng -> н, j -> дж, ch -> ч.
  Surnames: Kim -> Ким, Lee/Yi -> Ли, Park -> Пак, Choi -> Чхве, Jung/Jeong -> Чон, Kang -> Кан, Cho -> Чо, Yoon -> Юн, Jang -> Чан, Lim -> Им, Han -> Хан, Shin -> Син, Song -> Сон, Hwang -> Хван.
- A hyphenated or all-caps name (YE-ON, JI-HYO) is one name, never split into syllables or translated as words.
- Groups, fandoms, shows and brands stay in Latin as written: RESCENE, aespa, TWICE, ONCE.
- If a word would translate into an ordinary word but stands where a name fits, it is a name.
- Acronyms stay in Latin: MBTI, TMI, OOTD, PPL, ASMR.
- Honorifics in Cyrillic: онни, оппа, нуна, хён, макнэ, сонбэ(ним); "-ssi" -> "-сси".

3. Style
- Lively spoken Russian with the energy of the original; translate idioms, slang and jokes by meaning, not word-for-word.
- "ты" between members, friends and hosts; the audience is "вы" / "ребята".
- Short lines a viewer reads in 2–3 seconds; drop filler words instead of padding.
- Korean interjections get Russian equivalents: 대박 -> Офигеть, 헐 -> Чего?!, 아이고 -> Ой-ой, 화이팅 -> Вперёд!
- Mixed Korean and English in one line becomes one natural Russian sentence.
- Lyrics: meaning and mood in short lines, no forced rhyme.
- Keep swearing at the same strength; do not censor, soften or comment.
- Silently fix obvious typos and speech-recognition errors.

4. Example
[WONYOUNG] Wait, I really didn''t -> [WONYOUNG] Стоп, я правда совсем
expect that at all -> такого не ожидала
- Are you serious? - Yes! -> - Ты серьёзно? - Ага!
[Laughter] 대박 ♪ -> [Смех] Офигеть ♪', 'Custom prompt rules for AI subtitle translation')
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = CURRENT_TIMESTAMP;

UPDATE gemfactory.config SET key = 'SUBS_YOUTUBE_ONLY', description = 'Use YouTube auto-translate, skip LLM providers'
WHERE key = 'SUBS_GOOGLE_ONLY'
  AND NOT EXISTS (SELECT 1 FROM gemfactory.config WHERE key = 'SUBS_YOUTUBE_ONLY');

UPDATE gemfactory.config
SET value = array_to_string(array_remove(regexp_split_to_array(regexp_replace(value, '\s+', '', 'g'), ','), 'google'), ','),
    updated_at = CURRENT_TIMESTAMP
WHERE key = 'TRANSLATION_FALLBACK_ORDER' AND value ~ '(^|,)\s*google\s*(,|$)';

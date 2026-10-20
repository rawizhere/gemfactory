-- The previous prompt and fallback order are not restored: they may have been edited in the web panel.
UPDATE gemfactory.config SET key = 'SUBS_GOOGLE_ONLY', description = 'Use Google Translate, skip LLM providers'
WHERE key = 'SUBS_YOUTUBE_ONLY'
  AND NOT EXISTS (SELECT 1 FROM gemfactory.config WHERE key = 'SUBS_GOOGLE_ONLY');

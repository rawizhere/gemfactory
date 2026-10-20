-- Deleted provider keys are not restored; point the old fallback chain at the stored Gemini model.
INSERT INTO gemfactory.config (key, value, description)
SELECT 'GEMINI_MODELS', value, 'Comma-separated Gemini model fallback chain'
FROM gemfactory.config WHERE key = 'GEMINI_MODEL'
ON CONFLICT (key) DO NOTHING;
INSERT INTO gemfactory.config (key, value, description) VALUES
('TRANSLATION_FALLBACK_ORDER', 'gemini', 'Fallback order of translation providers')
ON CONFLICT (key) DO NOTHING;
DELETE FROM gemfactory.config WHERE key = 'GEMINI_MODEL';

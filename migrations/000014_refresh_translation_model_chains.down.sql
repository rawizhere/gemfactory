-- Revert model chains and fallback order to previous defaults.
INSERT INTO gemfactory.config (key, value, description) VALUES
('TRANSLATION_FALLBACK_ORDER', 'gemini,nvidia,groq,opencode,openrouter', 'Fallback order of translation providers'),
('OPENROUTER_MODELS', 'minimax/minimax-m3:free, nvidia/nemotron-3.5-lightning:free, dots-studio/dots-3-note-preview:free', 'Comma-separated OpenRouter model fallback chain'),
('OPENCODE_MODELS', 'x-preview-f-free, nemotron-3-ultra-free, big-pickle', 'Comma-separated OpenCode Zen model fallback chain'),
('NVIDIA_MODELS', 'minimaxai/minimax-m3', 'Comma-separated NVIDIA NIM model fallback chain'),
('GROQ_MODELS', 'groq/compound, openai/gpt-oss-120b', 'Comma-separated Groq model fallback chain')
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = CURRENT_TIMESTAMP;

-- Refresh stored model chains and fallback order to match active production defaults.
INSERT INTO gemfactory.config (key, value, description) VALUES
('TRANSLATION_FALLBACK_ORDER', 'openrouter,opencode,nvidia,groq', 'Fallback order of translation providers'),
('OPENROUTER_MODELS', 'minimax/minimax-m3:free, deepseek/deepseek-v4-flash-0731, nvidia/nemotron-3.5-lightning:free, dots-studio/dots-3-note-preview:free', 'Comma-separated OpenRouter model fallback chain'),
('OPENCODE_MODELS', 'laguna-s-2.1-free, nemotron-3.5-lightning-free, hy3-free', 'Comma-separated OpenCode Zen model fallback chain'),
('NVIDIA_MODELS', 'minimaxai/minimax-m3, openai/gpt-oss-120b, meta/muse-glimmer-30b, stepfun-ai/step-3.7-flash', 'Comma-separated NVIDIA NIM model fallback chain'),
('GROQ_MODELS', 'openai/gpt-oss-120b', 'Comma-separated Groq model fallback chain')
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = CURRENT_TIMESTAMP;

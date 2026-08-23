-- Refresh stored model chains to the current defaults.
INSERT INTO gemfactory.config (key, value, description) VALUES
('GROQ_MODELS', 'groq/compound, openai/gpt-oss-120b', 'Comma-separated Groq model fallback chain'),
('OPENCODE_MODELS', 'x-preview-f-free, nemotron-3-ultra-free, big-pickle', 'Comma-separated OpenCode Zen model fallback chain'),
('NVIDIA_MODELS', 'minimaxai/minimax-m3', 'Comma-separated NVIDIA NIM model fallback chain')
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = CURRENT_TIMESTAMP;

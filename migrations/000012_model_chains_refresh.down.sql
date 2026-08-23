-- Restore the model chains as seeded before this refresh.
INSERT INTO gemfactory.config (key, value, description) VALUES
('GROQ_MODELS', 'openai/gpt-oss-120b, qwen/qwen3.6-27b, groq/compound, openai/gpt-oss-20b', 'Comma-separated Groq model fallback chain'),
('OPENCODE_MODELS', 'big-pickle, laguna-s-2.1-free, x-preview-f-free, nemotron-3-ultra-free', 'Comma-separated OpenCode Zen model fallback chain'),
('NVIDIA_MODELS', 'minimaxai/minimax-m3', 'Comma-separated NVIDIA NIM model fallback chain')
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = CURRENT_TIMESTAMP;

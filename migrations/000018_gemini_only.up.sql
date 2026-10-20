-- Translation runs on Gemini only: drop the other providers and the fallback chain.
-- GEMINI_MODELS is not carried over; GEMINI_MODEL defaults to gemini-3.5-flash-lite in code.
DELETE FROM gemfactory.config WHERE key IN (
    'GROQ_API_KEY', 'OPENCODE_API_KEY', 'NVIDIA_API_KEY', 'OPENROUTER_API_KEY',
    'GEMINI_MODELS', 'GROQ_MODELS', 'OPENCODE_MODELS', 'NVIDIA_MODELS', 'OPENROUTER_MODELS',
    'TRANSLATION_FALLBACK_ORDER'
);

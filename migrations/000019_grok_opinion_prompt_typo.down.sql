UPDATE gemfactory.config SET value = replace(value, 'Сразу вердикт —', 'Сразу verdict —') WHERE key = 'GROK_OPINION_PROMPT';

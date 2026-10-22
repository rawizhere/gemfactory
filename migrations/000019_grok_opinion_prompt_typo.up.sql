UPDATE gemfactory.config SET value = replace(value, 'Сразу verdict —', 'Сразу вердикт —') WHERE key = 'GROK_OPINION_PROMPT';

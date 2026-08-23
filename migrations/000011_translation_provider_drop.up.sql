-- Primary provider is gone; the fallback chain alone drives provider selection.
DELETE FROM gemfactory.config WHERE key = 'TRANSLATION_PROVIDER';

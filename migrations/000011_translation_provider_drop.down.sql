-- Restore the primary provider setting with its last known default.
INSERT INTO gemfactory.config (key, value, description) VALUES
('TRANSLATION_PROVIDER', 'google', 'Primary subtitle translation provider')
ON CONFLICT (key) DO NOTHING;

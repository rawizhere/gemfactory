-- Setting for deleting or keeping status messages for /clip and /subs on completion
INSERT INTO gemfactory.config (key, value, description) VALUES
('CLIP_DELETE_STATUS', 'false', 'Delete status message for /clip and /subs on completion (true/false)')
ON CONFLICT (key) DO NOTHING;

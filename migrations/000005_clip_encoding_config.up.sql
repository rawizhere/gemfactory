-- Video encoding configuration for /clip and /subs
INSERT INTO gemfactory.config (key, value, description) VALUES
('CLIP_CRF', '20', 'CRF for /clip video re-encoding (lower = higher quality, 18-28)'),
('SUBS_CRF', '20', 'CRF for /subs video re-encoding with burned subtitles (18-28)'),
('CLIP_PRESET', 'fast', 'x264 preset for clip encoding (ultrafast, superfast, veryfast, faster, fast, medium)'),
('CLIP_AUDIO_BITRATE', '192k', 'Audio bitrate for clips (e.g. 128k, 192k, 256k, 320k)')
ON CONFLICT (key) DO NOTHING;

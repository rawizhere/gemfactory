-- Add display_artist column to releases table
ALTER TABLE gemfactory.releases ADD COLUMN IF NOT EXISTS display_artist VARCHAR(500);

-- Clean up typo/duplicate artists
DELETE FROM gemfactory.artists WHERE name = 'rescence';
DELETE FROM gemfactory.artists WHERE name = 'CLASSy';
DELETE FROM gemfactory.artists WHERE name = 'LE SSERAFIM x ILLIT x KATSEYE';
DELETE FROM gemfactory.artists WHERE name = 'Jay Park & LNGSHOT';

-- Reassign existing releases for group-member entries before deleting redundant artists
UPDATE gemfactory.releases r
SET artist_id = a_target.artist_id,
    display_artist = COALESCE(r.display_artist, 'MIMI (OH MY GIRL)')
FROM gemfactory.artists a_src, gemfactory.artists a_target
WHERE a_src.name = 'MIMI (OH MY GIRL)'
  AND a_target.name = 'OH MY GIRL'
  AND r.artist_id = a_src.artist_id;

UPDATE gemfactory.releases r
SET artist_id = a_target.artist_id,
    display_artist = COALESCE(r.display_artist, 'YUNA (ITZY)')
FROM gemfactory.artists a_src, gemfactory.artists a_target
WHERE a_src.name = 'YUNA (ITZY)'
  AND a_target.name = 'ITZY'
  AND r.artist_id = a_src.artist_id;

UPDATE gemfactory.releases r
SET artist_id = a_target.artist_id,
    display_artist = COALESCE(r.display_artist, 'Picheolin (SEVENTEEN Dino)')
FROM gemfactory.artists a_src, gemfactory.artists a_target
WHERE a_src.name = 'Picheolin (SEVENTEEN Dino)'
  AND a_target.name = 'Picheolin'
  AND r.artist_id = a_src.artist_id;

-- Delete redundant group-in-parenthesis artists
DELETE FROM gemfactory.artists WHERE name IN (
    'MIMI (OH MY GIRL)',
    'YUNA (ITZY)',
    'Picheolin (SEVENTEEN Dino)'
);

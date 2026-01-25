ALTER TABLE gemfactory.releases ADD COLUMN IF NOT EXISTS spotify VARCHAR(1000);
ALTER TABLE gemfactory.releases ADD COLUMN IF NOT EXISTS source_url VARCHAR(1000);

-- Fix column lengths for legacy databases
ALTER TABLE gemfactory.releases ALTER COLUMN title TYPE VARCHAR(500);
ALTER TABLE gemfactory.releases ALTER COLUMN title_track TYPE VARCHAR(500);
ALTER TABLE gemfactory.releases ALTER COLUMN album_name TYPE VARCHAR(500);
ALTER TABLE gemfactory.releases ALTER COLUMN mv TYPE VARCHAR(1000);
ALTER TABLE gemfactory.releases ALTER COLUMN date TYPE DATE USING date::date;

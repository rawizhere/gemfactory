-- GemFactory Consolidated Destruction Schema
-- Drops all tables and the schema.

SET search_path TO gemfactory, public;

DROP TABLE IF EXISTS gemfactory.playlist_tracks CASCADE;
DROP TABLE IF EXISTS gemfactory.homeworks CASCADE;
DROP TABLE IF EXISTS gemfactory.tasks CASCADE;
DROP TABLE IF EXISTS gemfactory.config CASCADE;
DROP TABLE IF EXISTS gemfactory.releases CASCADE;
DROP TABLE IF EXISTS gemfactory.artists CASCADE;

DROP SCHEMA IF EXISTS gemfactory CASCADE;

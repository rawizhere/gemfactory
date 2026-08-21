-- GemFactory Consolidated Destruction Schema
-- Drops all tables and the schema.

SET search_path TO gemfactory, public;

DROP TABLE IF EXISTS gemfactory.config CASCADE;
DROP TABLE IF EXISTS gemfactory.releases CASCADE;
DROP TABLE IF EXISTS gemfactory.artists CASCADE;

DROP SCHEMA IF EXISTS gemfactory CASCADE;

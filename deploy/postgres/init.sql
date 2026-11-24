-- Runs once on first PostgreSQL initialization.
\set ON_ERROR_STOP on

SELECT 'CREATE DATABASE gonews_test'
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'gonews_test')\gexec

ALTER DATABASE gonews SET timezone TO 'UTC';
ALTER DATABASE gonews_test SET timezone TO 'UTC';

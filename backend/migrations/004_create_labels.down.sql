-- Migration: 004_create_labels.down.sql
DROP FUNCTION IF EXISTS create_default_labels(UUID);
DROP TABLE IF EXISTS labels;

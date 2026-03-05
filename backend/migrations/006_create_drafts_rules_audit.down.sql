-- Migration: 006_create_drafts_rules_audit.down.sql
DROP TABLE IF EXISTS email_snoozes;
DROP TABLE IF EXISTS audit_logs;
DROP TABLE IF EXISTS email_rules;
DROP TABLE IF EXISTS drafts;

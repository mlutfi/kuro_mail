-- Migration: 001_create_users.down.sql
DROP TRIGGER IF EXISTS set_users_updated_at ON users;
DROP FUNCTION IF EXISTS trigger_set_updated_at();
DROP TABLE IF EXISTS users;
DROP EXTENSION IF EXISTS "pgcrypto";
DROP EXTENSION IF EXISTS "uuid-ossp";

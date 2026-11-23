-- Начальная схема MoloTalk. Источник — docs/hld/schema.sql, SHA-256
-- CD4201662F63B9D76BA4CEBA4C418F8B4274C03D57DD9D7968DC031ABA71ADCE.
--
-- Два принятых отличия от источника:
--   * исключён U06 UNIQUE messages(channel_id,id);
--   * C12 использует char_length(text) <= 4096 вместо length(text) <= 4000.

-- +goose Up
CREATE DOMAIN ulid AS char(26)
  CHECK (VALUE ~ '^[0-7][0-9A-HJKMNP-TV-Z]{25}$');

CREATE TABLE users (
  id ulid PRIMARY KEY,
  identifier text NOT NULL UNIQUE CHECK (length(identifier) BETWEEN 1 AND 254 AND identifier=lower(identifier)),
  display_name text NOT NULL CHECK (length(display_name) BETWEEN 1 AND 64 AND display_name ~ '[^[:space:]]'),
  avatar_attachment_id ulid UNIQUE,
  created_at timestamptz NOT NULL
);

CREATE TABLE user_auth (
  user_id ulid PRIMARY KEY REFERENCES users(id),
  password_hash text NOT NULL,
  password_changed_at timestamptz NOT NULL
);

CREATE TABLE sessions (
  id ulid PRIMARY KEY,
  user_id ulid NOT NULL REFERENCES users(id),
  created_at timestamptz NOT NULL,
  expires_at timestamptz NOT NULL,
  revoked_at timestamptz,
  CHECK (expires_at > created_at)
);

CREATE TABLE refresh_tokens (
  token_hash text PRIMARY KEY,
  session_id ulid NOT NULL REFERENCES sessions(id),
  expires_at timestamptz NOT NULL,
  consumed_at timestamptz
);

CREATE TABLE login_attempt_windows (
  identifier_hash char(64) PRIMARY KEY CHECK (identifier_hash ~ '^[0-9a-f]{64}$'),
  failed_count smallint NOT NULL CHECK (failed_count BETWEEN 1 AND 5),
  window_started_at timestamptz NOT NULL
);

CREATE TABLE channels (
  id ulid PRIMARY KEY,
  created_by_id ulid NOT NULL REFERENCES users(id),
  client_request_id ulid,
  create_request_hash text,
  kind text NOT NULL CHECK (kind IN ('DIRECT','GROUP')),
  name text CHECK (length(name) BETWEEN 1 AND 100),
  owner_id ulid REFERENCES users(id),
  direct_pair_key text UNIQUE,
  created_at timestamptz NOT NULL,
  closed_at timestamptz,
  UNIQUE (created_by_id,client_request_id),
  CHECK ((kind='DIRECT' AND client_request_id IS NULL AND create_request_hash IS NULL)
      OR (kind='GROUP' AND client_request_id IS NOT NULL AND create_request_hash IS NOT NULL)),
  CHECK ((kind='DIRECT' AND owner_id IS NULL AND direct_pair_key IS NOT NULL)
     OR (kind='GROUP' AND direct_pair_key IS NULL
         AND (closed_at IS NOT NULL OR (owner_id IS NOT NULL AND name IS NOT NULL AND name ~ '[^[:space:]]'))))
);

CREATE TABLE channel_members (
  channel_id ulid NOT NULL REFERENCES channels(id),
  user_id ulid NOT NULL REFERENCES users(id),
  role text NOT NULL DEFAULT 'MEMBER' CHECK (role IN ('ADMIN','MEMBER')),
  joined_at timestamptz NOT NULL,
  PRIMARY KEY (channel_id,user_id)
);

ALTER TABLE channels ADD CONSTRAINT group_owner_is_member
  FOREIGN KEY (id,owner_id) REFERENCES channel_members(channel_id,user_id)
  DEFERRABLE INITIALLY DEFERRED;

CREATE TABLE messages (
  id ulid PRIMARY KEY,
  channel_id ulid NOT NULL REFERENCES channels(id),
  author_id ulid NOT NULL REFERENCES users(id),
  client_message_id ulid NOT NULL,
  text text NOT NULL DEFAULT '' CHECK (char_length(text) <= 4096),
  attachment_ids varchar(26)[] NOT NULL DEFAULT '{}',
  revision integer NOT NULL CHECK (revision >= 1),
  created_at timestamptz NOT NULL,
  edited_at timestamptz,
  deleted_at timestamptz,
  request_hash text NOT NULL,
  UNIQUE (channel_id,author_id,client_message_id),
  CHECK (cardinality(attachment_ids) <= 5),
  CHECK (deleted_at IS NOT NULL OR text ~ '[^[:space:]]' OR cardinality(attachment_ids)>0)
);

CREATE TABLE message_receipts (
  message_id ulid NOT NULL REFERENCES messages(id),
  user_id ulid NOT NULL REFERENCES users(id),
  delivered_at timestamptz,
  read_at timestamptz,
  PRIMARY KEY (message_id,user_id),
  CHECK (read_at IS NULL OR (delivered_at IS NOT NULL AND read_at >= delivered_at))
);

CREATE TABLE attachments (
  id ulid PRIMARY KEY,
  uploader_id ulid NOT NULL REFERENCES users(id),
  purpose text NOT NULL CHECK (purpose IN ('AVATAR','MESSAGE_ATTACHMENT')),
  status text NOT NULL CHECK (status IN ('UPLOADING','VALIDATING','READY','REJECTED','DELETING')),
  object_key text NOT NULL UNIQUE,
  temp_object_key text UNIQUE,
  thumbnail_key text,
  filename text NOT NULL,
  content_type text NOT NULL,
  size_bytes bigint NOT NULL CHECK (size_bytes >= 0),
  channel_id ulid REFERENCES channels(id),
  linked_at timestamptz,
  created_at timestamptz NOT NULL,
  upload_expires_at timestamptz NOT NULL,
  completed_at timestamptz,
  CHECK (purpose <> 'AVATAR' OR size_bytes <= 5242880),
  CHECK (purpose <> 'MESSAGE_ATTACHMENT' OR size_bytes <= 20971520),
  CHECK ((channel_id IS NULL) = (linked_at IS NULL))
);

ALTER TABLE users ADD CONSTRAINT user_avatar_fk
  FOREIGN KEY (avatar_attachment_id) REFERENCES attachments(id);

CREATE INDEX member_user_idx ON channel_members(user_id,channel_id);
CREATE INDEX sessions_user_idx ON sessions(user_id);
CREATE INDEX refresh_session_idx ON refresh_tokens(session_id);
CREATE INDEX history_idx ON messages(channel_id,id DESC);
CREATE INDEX attachment_reference_idx ON messages USING gin(attachment_ids);
CREATE INDEX receipt_user_idx ON message_receipts(user_id,message_id);
CREATE INDEX attachment_cleanup_idx ON attachments(status,created_at) WHERE linked_at IS NULL;
CREATE INDEX receipt_unread_idx ON message_receipts(user_id,message_id) WHERE read_at IS NULL;
CREATE INDEX login_window_cleanup_idx ON login_attempt_windows(window_started_at);

-- +goose Down
DROP INDEX IF EXISTS login_window_cleanup_idx;
DROP INDEX IF EXISTS receipt_unread_idx;
DROP INDEX IF EXISTS attachment_cleanup_idx;
DROP INDEX IF EXISTS receipt_user_idx;
DROP INDEX IF EXISTS attachment_reference_idx;
DROP INDEX IF EXISTS history_idx;
DROP INDEX IF EXISTS refresh_session_idx;
DROP INDEX IF EXISTS sessions_user_idx;
DROP INDEX IF EXISTS member_user_idx;

ALTER TABLE users DROP CONSTRAINT IF EXISTS user_avatar_fk;
DROP TABLE IF EXISTS attachments;
DROP TABLE IF EXISTS message_receipts;
DROP TABLE IF EXISTS messages;
ALTER TABLE channels DROP CONSTRAINT IF EXISTS group_owner_is_member;
DROP TABLE IF EXISTS channel_members;
DROP TABLE IF EXISTS channels;
DROP TABLE IF EXISTS login_attempt_windows;
DROP TABLE IF EXISTS refresh_tokens;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS user_auth;
DROP TABLE IF EXISTS users;
DROP DOMAIN IF EXISTS ulid;

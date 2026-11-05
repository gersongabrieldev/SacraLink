-- Esquema inicial do SacraLink.
-- Convenção: timestamps em milissegundos desde a época Unix (INTEGER).

CREATE TABLE settings (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE TABLE users (
    id              INTEGER PRIMARY KEY,
    username        TEXT NOT NULL UNIQUE,
    password_hash   TEXT NOT NULL,
    role            TEXT NOT NULL DEFAULT 'admin',
    failed_attempts INTEGER NOT NULL DEFAULT 0,
    locked_until    INTEGER NOT NULL DEFAULT 0,
    created_at      INTEGER NOT NULL,
    updated_at      INTEGER NOT NULL
);

CREATE TABLE devices (
    id           TEXT PRIMARY KEY,
    name         TEXT NOT NULL,
    platform     TEXT NOT NULL CHECK (platform IN ('android', 'ios', 'other')),
    model        TEXT NOT NULL DEFAULT '',
    os_version   TEXT NOT NULL DEFAULT '',
    app_version  TEXT NOT NULL DEFAULT '',
    secret_hash  TEXT NOT NULL,
    status       TEXT NOT NULL CHECK (status IN ('active', 'revoked', 'blocked')),
    paired_at    INTEGER NOT NULL,
    last_seen_at INTEGER NOT NULL DEFAULT 0,
    last_ip      TEXT NOT NULL DEFAULT ''
);

CREATE TABLE device_permissions (
    device_id  TEXT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    permission TEXT NOT NULL,
    PRIMARY KEY (device_id, permission)
);

CREATE TABLE media_profiles (
    id               TEXT PRIMARY KEY,
    name             TEXT NOT NULL,
    width            INTEGER NOT NULL,
    height           INTEGER NOT NULL,
    fps              INTEGER NOT NULL,
    max_bitrate_kbps INTEGER NOT NULL,
    min_bitrate_kbps INTEGER NOT NULL,
    adaptive         INTEGER NOT NULL DEFAULT 1,
    builtin          INTEGER NOT NULL DEFAULT 0
);

-- Perfis iniciais sugeridos pela especificação (§9.3), sujeitos a testes.
INSERT INTO media_profiles (id, name, width, height, fps, max_bitrate_kbps, min_bitrate_kbps, adaptive, builtin) VALUES
    ('auto',        'Automático',          1280,  720, 30, 2500, 300, 1, 1),
    ('economico',   'Econômico 480p30',     854,  480, 30, 1000, 250, 1, 1),
    ('equilibrado', 'Equilibrado 720p30',  1280,  720, 30, 2500, 500, 1, 1),
    ('alta',        'Alta qualidade 1080p30', 1920, 1080, 30, 4500, 800, 1, 1),
    ('alta60',      'Alta qualidade 1080p60', 1920, 1080, 60, 6500, 1200, 1, 1);

CREATE TABLE cameras (
    id                   TEXT PRIMARY KEY,
    slot                 INTEGER NOT NULL UNIQUE CHECK (slot >= 1),
    name                 TEXT NOT NULL,
    operator_name        TEXT NOT NULL DEFAULT '',
    device_id            TEXT UNIQUE REFERENCES devices(id) ON DELETE SET NULL,
    profile_id           TEXT NOT NULL DEFAULT 'auto' REFERENCES media_profiles(id),
    ingest_mode          TEXT NOT NULL DEFAULT 'rtsp' CHECK (ingest_mode IN ('rtsp', 'whep')),
    is_backup            INTEGER NOT NULL DEFAULT 0,
    audio_sync_offset_ms INTEGER NOT NULL DEFAULT 0,
    created_at           INTEGER NOT NULL,
    updated_at           INTEGER NOT NULL
);

CREATE TABLE camera_presets (
    id         TEXT PRIMARY KEY,
    camera_id  TEXT NOT NULL REFERENCES cameras(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    params     TEXT NOT NULL,
    created_at INTEGER NOT NULL
);

CREATE TABLE pairing_tokens (
    token_hash     TEXT PRIMARY KEY,
    label          TEXT NOT NULL DEFAULT '',
    camera_id      TEXT NOT NULL DEFAULT '',
    created_at     INTEGER NOT NULL,
    expires_at     INTEGER NOT NULL,
    used_at        INTEGER NOT NULL DEFAULT 0,
    used_by_device TEXT NOT NULL DEFAULT ''
);

CREATE TABLE obs_connections (
    id              INTEGER PRIMARY KEY CHECK (id = 1),
    host            TEXT NOT NULL,
    port            INTEGER NOT NULL,
    password_enc    BLOB,
    auto_connect    INTEGER NOT NULL DEFAULT 1,
    last_obs_version TEXT NOT NULL DEFAULT '',
    last_ws_version  TEXT NOT NULL DEFAULT '',
    updated_at      INTEGER NOT NULL
);

CREATE TABLE obs_source_mappings (
    camera_id  TEXT PRIMARY KEY REFERENCES cameras(id) ON DELETE CASCADE,
    input_name TEXT NOT NULL,
    input_uuid TEXT NOT NULL DEFAULT '',
    input_kind TEXT NOT NULL,
    scene_name TEXT NOT NULL,
    scene_uuid TEXT NOT NULL DEFAULT '',
    updated_at INTEGER NOT NULL
);

CREATE TABLE stream_sessions (
    id             TEXT PRIMARY KEY,
    camera_id      TEXT NOT NULL REFERENCES cameras(id) ON DELETE CASCADE,
    started_at     INTEGER NOT NULL,
    ended_at       INTEGER NOT NULL DEFAULT 0,
    end_reason     TEXT NOT NULL DEFAULT '',
    remote_addr    TEXT NOT NULL DEFAULT '',
    bytes_received INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX idx_stream_sessions_camera ON stream_sessions(camera_id, started_at);

CREATE TABLE events (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    ts        INTEGER NOT NULL,
    level     TEXT NOT NULL,
    kind      TEXT NOT NULL,
    camera_id TEXT NOT NULL DEFAULT '',
    message   TEXT NOT NULL,
    data      TEXT NOT NULL DEFAULT '{}'
);
CREATE INDEX idx_events_ts ON events(ts);

CREATE TABLE connection_metrics (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    ts           INTEGER NOT NULL,
    camera_id    TEXT NOT NULL,
    bitrate_kbps REAL NOT NULL DEFAULT 0,
    fps          REAL NOT NULL DEFAULT 0,
    rtt_ms       REAL NOT NULL DEFAULT 0,
    packet_loss  REAL NOT NULL DEFAULT 0,
    battery      REAL NOT NULL DEFAULT -1,
    thermal      TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_connection_metrics_ts ON connection_metrics(ts);
CREATE INDEX idx_connection_metrics_camera ON connection_metrics(camera_id, ts);

CREATE TABLE audit_logs (
    id      INTEGER PRIMARY KEY AUTOINCREMENT,
    ts      INTEGER NOT NULL,
    actor   TEXT NOT NULL,
    action  TEXT NOT NULL,
    target  TEXT NOT NULL DEFAULT '',
    ip      TEXT NOT NULL DEFAULT '',
    success INTEGER NOT NULL,
    details TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_audit_logs_ts ON audit_logs(ts);

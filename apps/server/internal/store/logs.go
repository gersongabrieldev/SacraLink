package store

import (
	"context"
	"encoding/json"
	"time"
)

// Event é um evento operacional exibido no painel (queda de câmera, OBS
// desconectado, troca automática etc.).
type Event struct {
	ID       int64           `json:"id"`
	TS       int64           `json:"ts"`
	Level    string          `json:"level"`
	Kind     string          `json:"kind"`
	CameraID string          `json:"cameraId,omitempty"`
	Message  string          `json:"message"`
	Data     json.RawMessage `json:"data,omitempty"`
}

// AddEvent grava um evento operacional.
func (s *Store) AddEvent(ctx context.Context, level, kind, cameraID, message string, data any) (*Event, error) {
	raw := []byte("{}")
	if data != nil {
		b, err := json.Marshal(data)
		if err != nil {
			return nil, err
		}
		raw = b
	}
	ts := s.nowMs()
	res, err := s.db.ExecContext(ctx, `INSERT INTO events (ts, level, kind, camera_id, message, data) VALUES (?, ?, ?, ?, ?, ?)`,
		ts, level, kind, cameraID, message, string(raw))
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return &Event{ID: id, TS: ts, Level: level, Kind: kind, CameraID: cameraID, Message: message, Data: raw}, nil
}

// ListEvents retorna os eventos mais recentes (até limit).
func (s *Store) ListEvents(ctx context.Context, limit int) ([]*Event, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, ts, level, kind, camera_id, message, data FROM events ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Event{}
	for rows.Next() {
		e := &Event{}
		var data string
		if err := rows.Scan(&e.ID, &e.TS, &e.Level, &e.Kind, &e.CameraID, &e.Message, &data); err != nil {
			return nil, err
		}
		e.Data = json.RawMessage(data)
		out = append(out, e)
	}
	return out, rows.Err()
}

// AuditEntry é um registro de auditoria de segurança.
type AuditEntry struct {
	ID      int64  `json:"id"`
	TS      int64  `json:"ts"`
	Actor   string `json:"actor"`
	Action  string `json:"action"`
	Target  string `json:"target"`
	IP      string `json:"ip"`
	Success bool   `json:"success"`
	Details string `json:"details"`
}

// Audit grava um registro de auditoria. Falhas de gravação são retornadas,
// mas o chamador normalmente apenas registra no log.
func (s *Store) Audit(ctx context.Context, actor, action, target, ip string, success bool, details string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO audit_logs (ts, actor, action, target, ip, success, details) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		s.nowMs(), actor, action, target, ip, boolToInt(success), details)
	return err
}

// ListAudit retorna os registros de auditoria mais recentes.
func (s *Store) ListAudit(ctx context.Context, limit int) ([]*AuditEntry, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, ts, actor, action, target, ip, success, details FROM audit_logs ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*AuditEntry{}
	for rows.Next() {
		a := &AuditEntry{}
		var ok int
		if err := rows.Scan(&a.ID, &a.TS, &a.Actor, &a.Action, &a.Target, &a.IP, &ok, &a.Details); err != nil {
			return nil, err
		}
		a.Success = ok == 1
		out = append(out, a)
	}
	return out, rows.Err()
}

// Metric é uma amostra de métricas de conexão de uma câmera.
type Metric struct {
	TS          int64   `json:"ts"`
	CameraID    string  `json:"cameraId"`
	BitrateKbps float64 `json:"bitrateKbps"`
	FPS         float64 `json:"fps"`
	RTTMs       float64 `json:"rttMs"`
	PacketLoss  float64 `json:"packetLoss"`
	Battery     float64 `json:"battery"`
	Thermal     string  `json:"thermal"`
}

// AddMetric grava uma amostra de métricas.
func (s *Store) AddMetric(ctx context.Context, m Metric) error {
	if m.TS == 0 {
		m.TS = s.nowMs()
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO connection_metrics (ts, camera_id, bitrate_kbps, fps, rtt_ms, packet_loss, battery, thermal)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, m.TS, m.CameraID, m.BitrateKbps, m.FPS, m.RTTMs, m.PacketLoss, m.Battery, m.Thermal)
	return err
}

// ListMetrics retorna as amostras de uma câmera desde sinceMs.
func (s *Store) ListMetrics(ctx context.Context, cameraID string, sinceMs int64) ([]Metric, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT ts, camera_id, bitrate_kbps, fps, rtt_ms, packet_loss, battery, thermal
		FROM connection_metrics WHERE camera_id = ? AND ts >= ? ORDER BY ts LIMIT 5000`, cameraID, sinceMs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Metric{}
	for rows.Next() {
		var m Metric
		if err := rows.Scan(&m.TS, &m.CameraID, &m.BitrateKbps, &m.FPS, &m.RTTMs, &m.PacketLoss, &m.Battery, &m.Thermal); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// RetentionPolicy define por quanto tempo cada tipo de registro é mantido.
type RetentionPolicy struct {
	Events   time.Duration
	Metrics  time.Duration
	Audit    time.Duration
	Sessions time.Duration
}

// DefaultRetention é a política padrão de retenção.
var DefaultRetention = RetentionPolicy{
	Events:   30 * 24 * time.Hour,
	Metrics:  7 * 24 * time.Hour,
	Audit:    180 * 24 * time.Hour,
	Sessions: 90 * 24 * time.Hour,
}

// ApplyRetention remove registros mais antigos que a política.
func (s *Store) ApplyRetention(ctx context.Context, p RetentionPolicy) error {
	now := s.nowMs()
	stmts := []struct {
		q   string
		age time.Duration
	}{
		{`DELETE FROM events WHERE ts < ?`, p.Events},
		{`DELETE FROM connection_metrics WHERE ts < ?`, p.Metrics},
		{`DELETE FROM audit_logs WHERE ts < ?`, p.Audit},
		{`DELETE FROM stream_sessions WHERE ended_at > 0 AND ended_at < ?`, p.Sessions},
	}
	for _, st := range stmts {
		if _, err := s.db.ExecContext(ctx, st.q, now-st.age.Milliseconds()); err != nil {
			return err
		}
	}
	return s.PurgeExpiredPairingTokens(ctx)
}

// StreamSession registra uma sessão de transmissão de uma câmera.
type StreamSession struct {
	ID            string `json:"id"`
	CameraID      string `json:"cameraId"`
	StartedAt     int64  `json:"startedAt"`
	EndedAt       int64  `json:"endedAt"`
	EndReason     string `json:"endReason"`
	RemoteAddr    string `json:"remoteAddr"`
	BytesReceived int64  `json:"bytesReceived"`
}

// StartStreamSession abre uma sessão de transmissão.
func (s *Store) StartStreamSession(ctx context.Context, id, cameraID, remoteAddr string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO stream_sessions (id, camera_id, started_at, remote_addr) VALUES (?, ?, ?, ?)`,
		id, cameraID, s.nowMs(), remoteAddr)
	return err
}

// EndStreamSession fecha uma sessão de transmissão.
func (s *Store) EndStreamSession(ctx context.Context, id, reason string, bytesReceived int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE stream_sessions SET ended_at = ?, end_reason = ?, bytes_received = ? WHERE id = ? AND ended_at = 0`,
		s.nowMs(), reason, bytesReceived, id)
	return err
}

// CloseDanglingSessions fecha sessões que ficaram abertas após um encerramento
// inesperado do servidor.
func (s *Store) CloseDanglingSessions(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `UPDATE stream_sessions SET ended_at = ?, end_reason = 'server_restart' WHERE ended_at = 0`, s.nowMs())
	return err
}

// ListStreamSessions lista as sessões mais recentes.
func (s *Store) ListStreamSessions(ctx context.Context, limit int) ([]*StreamSession, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, camera_id, started_at, ended_at, end_reason, remote_addr, bytes_received
		FROM stream_sessions ORDER BY started_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*StreamSession{}
	for rows.Next() {
		ss := &StreamSession{}
		if err := rows.Scan(&ss.ID, &ss.CameraID, &ss.StartedAt, &ss.EndedAt, &ss.EndReason, &ss.RemoteAddr, &ss.BytesReceived); err != nil {
			return nil, err
		}
		out = append(out, ss)
	}
	return out, rows.Err()
}

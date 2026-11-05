package core

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gersongabrieldev/sacralink/apps/server/internal/control"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/devices"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/obsbridge"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/settings"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/store"
)

// profileMap converte um perfil de mídia na configuração enviada ao celular.
func profileMap(p *store.MediaProfile) map[string]any {
	return map[string]any{
		"id": p.ID, "name": p.Name, "width": p.Width, "height": p.Height, "fps": p.FPS,
		"maxBitrateKbps": p.MaxBitrateKbps, "minBitrateKbps": p.MinBitrateKbps, "adaptive": p.Adaptive,
	}
}

// OnHello implementa control.Handler.
func (c *Core) OnHello(p control.Peer, sessionID string, h control.Hello) (control.HelloAck, error) {
	ctx := context.Background()
	cam, err := c.opt.Store.GetCamera(ctx, p.CameraID)
	if err != nil {
		return control.HelloAck{}, err
	}
	prof, err := c.opt.Store.GetProfile(ctx, cam.ProfileID)
	if err != nil {
		prof, _ = c.opt.Store.GetProfile(ctx, "auto")
	}
	_ = c.opt.Store.TouchDevice(ctx, p.DeviceID, p.RemoteAddr, h.AppVersion, h.OSVersion)
	c.mu.Lock()
	_, known := c.cams[p.CameraID]
	c.mu.Unlock()
	if !known {
		_ = c.Reload(ctx)
	}
	caps := h.Capabilities
	c.mu.Lock()
	if s := c.cams[p.CameraID]; s != nil {
		s.controlConnected, s.connectedSince, s.remoteAddr, s.sessionID = true, time.Now(), p.RemoteAddr, sessionID
		s.caps = &caps
		if s.device != nil {
			s.device.AppVersion = h.AppVersion
		}
		c.dirty[p.CameraID] = true
	}
	c.mu.Unlock()
	c.event(ctx, "info", "camera_connected", p.CameraID, cam.Name+" conectou ao servidor")
	ack := control.HelloAck{
		DeviceID: p.DeviceID, CameraID: cam.ID, CameraName: cam.Name, Slot: cam.Slot,
		ServerName: c.opt.Settings.Get().ServerName, WHIPPath: "/device/v1/whip",
	}
	if prof != nil {
		ack.Profile = profileMap(prof)
	}
	return ack, nil
}

// OnTelemetry implementa control.Handler.
func (c *Core) OnTelemetry(p control.Peer, t control.Telemetry) {
	c.mu.Lock()
	s := c.cams[p.CameraID]
	if s == nil {
		c.mu.Unlock()
		return
	}
	s.tel = t
	c.dirty[p.CameraID] = true
	store10s := time.Since(s.lastMetric) >= 10*time.Second
	if store10s {
		s.lastMetric = time.Now()
	}
	m := store.Metric{CameraID: p.CameraID, BitrateKbps: s.bitrateKbps, Battery: -1}
	if t.Stream != nil {
		m.FPS, m.RTTMs, m.PacketLoss = t.Stream.FPS, t.Stream.RTTMs, float64(t.Stream.PacketsLost)
	}
	if t.Battery != nil {
		m.Battery = t.Battery.Level
	}
	m.Thermal = t.Thermal
	c.mu.Unlock()
	if store10s {
		_ = c.opt.Store.AddMetric(context.Background(), m)
	}
	if t.Thermal == "critical" {
		c.event(context.Background(), "warn", "camera_thermal", p.CameraID, "aparelho em temperatura crítica: o celular deve reduzir a qualidade")
	}
}

// OnLocalChange implementa control.Handler.
func (c *Core) OnLocalChange(p control.Peer, ch control.LocalChange) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if s := c.cams[p.CameraID]; s != nil {
		s.lastLocal = &LocalChangeView{Param: ch.Param, At: time.Now().UnixMilli()}
		c.dirty[p.CameraID] = true
	}
}

// OnCommandResult implementa control.Handler (o resultado também é retornado
// diretamente a quem enviou o comando).
func (c *Core) OnCommandResult(control.Peer, control.CommandResult) {}

// OnDisconnect implementa control.Handler.
func (c *Core) OnDisconnect(p control.Peer, sessionID, reason string) {
	c.mu.Lock()
	s := c.cams[p.CameraID]
	name := ""
	if s != nil && s.sessionID == sessionID {
		s.controlConnected, s.sessionID, s.tel = false, "", control.Telemetry{}
		name = s.cam.Name
		c.dirty[p.CameraID] = true
	}
	c.mu.Unlock()
	if name != "" {
		c.event(context.Background(), "warn", "camera_disconnected", p.CameraID, fmt.Sprintf("%s desconectou (%s)", name, reason))
	}
}

// Command envia um comando remoto a uma câmera e registra o resultado.
func (c *Core) Command(ctx context.Context, actor, cameraID, name string, params map[string]any, force bool) (control.CommandResult, error) {
	cam, err := c.opt.Store.GetCamera(ctx, cameraID)
	if err != nil {
		return control.CommandResult{}, ErrUnknownCamera
	}
	if cam.DeviceID == "" {
		return control.CommandResult{Status: control.ResultOffline, Message: "câmera sem aparelho associado"}, nil
	}
	dev, err := c.opt.Store.GetDevice(ctx, cam.DeviceID)
	if err != nil {
		return control.CommandResult{}, err
	}
	if !devices.HasPermission(dev, store.PermRemoteCtrl) {
		return control.CommandResult{}, ErrNoPermission
	}
	if params == nil {
		params = map[string]any{}
	}
	// Resolve perfis e presets no servidor para que o app não precise conhecê-los.
	switch name {
	case control.CmdQualitySet:
		if id, ok := params["profileId"].(string); ok {
			prof, err := c.opt.Store.GetProfile(ctx, id)
			if err != nil {
				return control.CommandResult{}, fmt.Errorf("%w: perfil desconhecido", control.ErrInvalidCommand)
			}
			for k, v := range profileMap(prof) {
				params[k] = v
			}
			params["profileId"] = id
		}
	}
	if c.ctrl == nil {
		return control.CommandResult{Status: control.ResultOffline}, nil
	}
	res, err := c.ctrl.SendCommand(ctx, cameraID, name, params, force)
	if err != nil {
		return res, err
	}
	c.mu.Lock()
	if s := c.cams[cameraID]; s != nil {
		s.lastCommand = &CommandView{CommandID: res.CommandID, Name: name, Status: res.Status, Message: res.Message, At: time.Now().UnixMilli()}
		c.dirty[cameraID] = true
	}
	c.mu.Unlock()
	_ = c.opt.Store.Audit(ctx, actor, "camera.command."+name, cameraID, "", res.Status == control.ResultExecuted, res.Status)
	if name == control.CmdQualitySet && res.Status == control.ResultExecuted {
		if id, ok := params["profileId"].(string); ok {
			_, _ = c.opt.Store.UpdateCamera(ctx, cameraID, store.CameraPatch{ProfileID: &id})
			_ = c.Reload(ctx)
		}
	}
	if c.opt.Events != nil {
		c.opt.Events.Publish("command", map[string]any{"cameraId": cameraID, "name": name, "result": res})
	}
	return res, nil
}

// UpdateCamera altera dados da câmera e propaga ao OBS e ao celular.
func (c *Core) UpdateCamera(ctx context.Context, actor, id string, p store.CameraPatch) (*store.Camera, error) {
	before, err := c.opt.Store.GetCamera(ctx, id)
	if err != nil {
		return nil, err
	}
	cam, err := c.opt.Store.UpdateCamera(ctx, id, p)
	if err != nil {
		return nil, err
	}
	_ = c.Reload(ctx)
	_ = c.opt.Store.Audit(ctx, actor, "camera.update", id, "", true, "")
	if c.obs != nil && cam.Name != before.Name {
		if err := c.obs.RenameCameraSource(ctx, refOf(*cam)); err != nil && !errors.Is(err, obsbridge.ErrNotConnected) && !errors.Is(err, obsbridge.ErrNoSource) {
			c.log.Warn("falha ao renomear fonte no OBS", "erro", err)
		}
	}
	if c.ctrl != nil {
		cfg := map[string]any{"cameraName": cam.Name}
		if prof, err := c.opt.Store.GetProfile(ctx, cam.ProfileID); err == nil {
			cfg["profile"] = profileMap(prof)
		}
		c.ctrl.SendConfig(id, cfg)
	}
	return cam, nil
}

// DisconnectDevice derruba controle e mídia do aparelho de uma câmera.
func (c *Core) DisconnectDevice(ctx context.Context, cameraID, reason string) {
	if c.ctrl != nil {
		c.ctrl.Disconnect(cameraID, reason)
	}
	cam, err := c.opt.Store.GetCamera(ctx, cameraID)
	if err != nil {
		return
	}
	if sessions, err := c.opt.Media.WebRTCSessions(ctx); err == nil {
		for _, s := range sessions {
			if s.Path == cam.MediaPath() && s.State == "publish" {
				_ = c.opt.Media.KickWebRTCSession(ctx, s.ID)
			}
		}
	}
}

// scheduleFallback aplica a política configurada quando a câmera NO AR cai.
// O Program NUNCA é alterado se o operador não habilitou troca automática.
func (c *Core) scheduleFallback(cameraID string) {
	cfg := c.opt.Settings.Get().Fallback
	if cfg.Mode == settings.FallbackStay || c.obs == nil {
		return
	}
	c.mu.Lock()
	if old := c.fallbackAt[cameraID]; old != nil {
		old.Stop()
	}
	c.fallbackAt[cameraID] = time.AfterFunc(time.Duration(cfg.GraceSec)*time.Second, func() {
		c.runFallback(context.Background(), cameraID, cfg)
	})
	c.mu.Unlock()
}

func (c *Core) runFallback(ctx context.Context, cameraID string, cfg settings.Fallback) {
	c.mu.Lock()
	delete(c.fallbackAt, cameraID)
	s := c.cams[cameraID]
	if s == nil || s.state == StateStreaming || !s.tally.Program {
		c.mu.Unlock()
		return // voltou a tempo ou já saiu do ar
	}
	name := s.cam.Name
	var backupID string
	if cfg.Mode == settings.FallbackBackup {
		for id, o := range c.cams {
			if id != cameraID && o.cam.IsBackup && o.state == StateStreaming {
				backupID = id
				break
			}
		}
	}
	c.mu.Unlock()

	var scene string
	switch cfg.Mode {
	case settings.FallbackBackup:
		if backupID == "" {
			c.event(ctx, "error", "fallback_failed", cameraID, "nenhuma câmera reserva transmitindo; o Program não foi alterado")
			return
		}
		var err error
		if scene, err = c.obs.CameraSceneName(backupID); err != nil {
			c.event(ctx, "error", "fallback_failed", cameraID, "a câmera reserva não tem cena no OBS; o Program não foi alterado")
			return
		}
	case settings.FallbackScene:
		scene = cfg.SceneName
	}
	if err := c.obs.ForceProgramScene(ctx, scene); err != nil {
		c.event(ctx, "error", "fallback_failed", cameraID, "falha na troca automática: "+err.Error())
		return
	}
	_ = c.opt.Store.Audit(ctx, "sistema", "obs.fallback", cameraID, "", true, scene)
	c.event(ctx, "warn", "fallback_applied", cameraID, fmt.Sprintf("troca automática: %s caiu, Program alterado para %q", name, scene))
}

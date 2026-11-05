package obsbridge

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/gersongabrieldev/sacralink/apps/server/internal/obs"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/store"
)

type storeMapping = store.SourceMapping

// Tipos de entrada do OBS usados pelo SacraLink.
const (
	KindMediaSource   = "ffmpeg_source"
	KindBrowserSource = "browser_source"
)

// Prefixo dos nomes de fontes e cenas criadas pelo SacraLink.
const NamePrefix = "SacraLink — "

// InputName retorna o nome padrão da fonte de uma câmera.
func InputName(cam CameraRef) string { return NamePrefix + cam.Name }

// SceneName retorna o nome padrão da cena dedicada de uma câmera.
func SceneName(cam CameraRef) string { return NamePrefix + "Cena " + cam.Name }

// desiredInput retorna o tipo e as configurações da fonte conforme o modo de
// ingestão (ADR-0004).
func (b *Bridge) desiredInput(cam CameraRef, canvasW, canvasH int) (string, map[string]any) {
	if cam.IngestMode == store.IngestWHEP {
		if canvasW == 0 {
			canvasW, canvasH = 1920, 1080
		}
		return KindBrowserSource, map[string]any{
			"url":                 b.opt.PlayerBase + cam.ID,
			"width":               canvasW,
			"height":              canvasH,
			"fps_custom":          false,
			"reroute_audio":       true, // "Controlar áudio pelo OBS"
			"restart_when_active": false,
			"shutdown":            false,
		}
	}
	return KindMediaSource, map[string]any{
		"is_local_file":       false,
		"input":               b.opt.RTSPBase + cam.MediaPath,
		"ffmpeg_options":      "rtsp_transport=tcp fflags=nobuffer",
		"buffering_mb":        1,
		"reconnect_delay_sec": 1,
		"restart_on_activate": false,
		"close_when_inactive": false,
		"hw_decode":           true,
	}
}

// refersTo informa se as configurações de uma entrada apontam para a câmera
// (usado para adotar fontes existentes após reinstalação/perda do banco).
func refersTo(settings map[string]any, cam CameraRef) bool {
	for _, k := range []string{"input", "url"} {
		if v, ok := settings[k].(string); ok && strings.Contains(v, cam.ID) {
			return true
		}
	}
	return false
}

// EnsureOptions controla EnsureSource.
type EnsureOptions struct {
	// Recreate permite recriar uma fonte removida pelo usuário ou de outro
	// modo de ingestão (ação explícita do operador).
	Recreate bool
}

// EnsureSource garante que a câmera tenha fonte e cena no OBS. É idempotente:
// reconexões não duplicam fontes. Nunca altera fontes que o SacraLink não
// criou.
func (b *Bridge) EnsureSource(ctx context.Context, cam CameraRef, opt EnsureOptions) (*store.SourceMapping, error) {
	c := b.currentClient()
	if c == nil {
		return nil, ErrNotConnected
	}
	b.mu.Lock()
	canvasW, canvasH := b.status.CanvasWidth, b.status.CanvasHeight
	b.mu.Unlock()
	kind, settings := b.desiredInput(cam, canvasW, canvasH)

	inputs, err := c.GetInputList(ctx)
	if err != nil {
		return nil, err
	}
	m, err := b.opt.Store.GetSourceMapping(ctx, cam.ID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	var found *obs.Input
	if m != nil {
		for i := range inputs {
			if (m.InputUUID != "" && inputs[i].InputUUID == m.InputUUID) || inputs[i].InputName == m.InputName {
				found = &inputs[i]
				break
			}
		}
	} else {
		// Sem mapeamento: adota uma fonte com o nome padrão que já aponte para
		// esta câmera (ex.: banco recriado). Fontes de terceiros não são tocadas.
		for i := range inputs {
			if inputs[i].InputName != InputName(cam) {
				continue
			}
			if s, _, err := c.GetInputSettings(ctx, inputs[i].InputName); err == nil && refersTo(s, cam) {
				found = &inputs[i]
				m = &store.SourceMapping{CameraID: cam.ID, SceneName: SceneName(cam)}
			}
			break
		}
	}

	if found != nil && found.InputKind != kind {
		if !opt.Recreate {
			return nil, ErrIngestModeMismatch
		}
		if err := c.RemoveInput(ctx, found.InputName); err != nil { // fonte criada pelo SacraLink
			return nil, fmt.Errorf("remover fonte antiga: %w", err)
		}
		found = nil
	}

	if found != nil {
		if err := c.SetInputSettings(ctx, found.InputName, settings, true); err != nil {
			return nil, err
		}
		if cam.AudioSyncOffsetMs != 0 {
			_ = c.SetInputAudioSyncOffset(ctx, found.InputName, cam.AudioSyncOffsetMs)
		}
		m.InputName, m.InputUUID, m.InputKind = found.InputName, found.InputUUID, found.InputKind
		return m, b.saveMapping(ctx, m, true)
	}

	if m != nil && !opt.Recreate {
		b.mu.Lock()
		b.mappings[cam.ID] = m
		b.present[cam.ID] = false
		b.mu.Unlock()
		return nil, ErrSourceRemovedByUser
	}
	return b.createSource(ctx, c, cam, kind, settings, m)
}

func (b *Bridge) createSource(ctx context.Context, c *obs.Client, cam CameraRef, kind string, settings map[string]any, prev *store.SourceMapping) (*store.SourceMapping, error) {
	scenes, err := c.GetSceneList(ctx)
	if err != nil {
		return nil, err
	}
	inputs, err := c.GetInputList(ctx)
	if err != nil {
		return nil, err
	}
	taken := map[string]bool{}
	sceneExists := map[string]string{}
	for _, s := range scenes.Scenes {
		taken[s.SceneName] = true
		sceneExists[s.SceneName] = s.SceneUUID
	}
	for _, in := range inputs {
		taken[in.InputName] = true
	}

	// Cena dedicada: reutiliza a do mapeamento anterior se ainda existir.
	sceneName, sceneUUID := "", ""
	if prev != nil && prev.SceneName != "" {
		if u, ok := sceneExists[prev.SceneName]; ok {
			sceneName, sceneUUID = prev.SceneName, u
		}
	}
	if sceneName == "" {
		sceneName = uniqueName(SceneName(cam), taken)
		taken[sceneName] = true
		if sceneUUID, err = c.CreateScene(ctx, sceneName); err != nil {
			return nil, fmt.Errorf("criar cena %q: %w", sceneName, err)
		}
	}
	inputName := uniqueName(InputName(cam), taken)
	created, err := c.CreateInput(ctx, sceneName, inputName, kind, settings, true)
	if err != nil {
		return nil, fmt.Errorf("criar fonte %q: %w", inputName, err)
	}
	b.mu.Lock()
	w, h := b.status.CanvasWidth, b.status.CanvasHeight
	b.mu.Unlock()
	if w > 0 && h > 0 {
		// Ajusta a imagem ao canvas independentemente da resolução do celular.
		_ = c.SetSceneItemTransform(ctx, sceneName, created.SceneItemID, map[string]any{
			"positionX": 0, "positionY": 0, "alignment": 5,
			"boundsType": "OBS_BOUNDS_SCALE_INNER", "boundsAlignment": 0,
			"boundsWidth": w, "boundsHeight": h,
		})
	}
	if cam.AudioSyncOffsetMs != 0 {
		_ = c.SetInputAudioSyncOffset(ctx, inputName, cam.AudioSyncOffsetMs)
	}
	m := &store.SourceMapping{
		CameraID: cam.ID, InputName: inputName, InputUUID: created.InputUUID, InputKind: kind,
		SceneName: sceneName, SceneUUID: sceneUUID,
	}
	b.log.Info("fonte criada no OBS", "camera", cam.ID, "fonte", inputName, "cena", sceneName)
	return m, b.saveMapping(ctx, m, true)
}

func (b *Bridge) saveMapping(ctx context.Context, m *store.SourceMapping, present bool) error {
	if err := b.opt.Store.UpsertSourceMapping(ctx, m); err != nil {
		return err
	}
	b.mu.Lock()
	cp := *m
	b.mappings[m.CameraID] = &cp
	b.present[m.CameraID] = present
	b.mu.Unlock()
	b.emitTally()
	b.notifyChange()
	return nil
}

func uniqueName(base string, taken map[string]bool) string {
	if !taken[base] {
		return base
	}
	for i := 2; ; i++ {
		n := fmt.Sprintf("%s (%d)", base, i)
		if !taken[n] {
			return n
		}
	}
}

// SyncCameras garante as fontes de todas as câmeras sem mapeamento ou
// presentes no OBS (fontes removidas pelo usuário não são recriadas).
func (b *Bridge) SyncCameras(ctx context.Context, cams []CameraRef) {
	for _, cam := range cams {
		if _, err := b.EnsureSource(ctx, cam, EnsureOptions{}); err != nil &&
			!errors.Is(err, ErrSourceRemovedByUser) && !errors.Is(err, ErrNotConnected) {
			b.log.Warn("falha ao garantir fonte da câmera", "camera", cam.ID, "erro", err)
		}
	}
}

// RenameCameraSource renomeia a fonte e a cena de uma câmera (gerenciadas
// pelo SacraLink) para refletir o novo nome da câmera.
func (b *Bridge) RenameCameraSource(ctx context.Context, cam CameraRef) error {
	c := b.currentClient()
	if c == nil {
		return ErrNotConnected
	}
	b.mu.Lock()
	m := b.mappings[cam.ID]
	present := b.present[cam.ID]
	b.mu.Unlock()
	if m == nil || !present {
		return ErrNoSource
	}
	cp := *m
	if want := InputName(cam); cp.InputName != want {
		if err := c.SetInputName(ctx, cp.InputName, want); err != nil {
			return err
		}
		cp.InputName = want
	}
	if want := SceneName(cam); cp.SceneName != "" && cp.SceneName != want {
		if err := c.SetSceneName(ctx, cp.SceneName, want); err == nil {
			cp.SceneName = want
		}
	}
	return b.saveMapping(ctx, &cp, true)
}

// Mapping retorna o mapeamento de uma câmera (nil se não houver).
func (b *Bridge) Mapping(cameraID string) *store.SourceMapping {
	b.mu.Lock()
	defer b.mu.Unlock()
	if m := b.mappings[cameraID]; m != nil {
		cp := *m
		return &cp
	}
	return nil
}

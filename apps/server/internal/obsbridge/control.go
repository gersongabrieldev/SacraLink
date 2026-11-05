package obsbridge

import (
	"context"
	"sort"

	"github.com/gersongabrieldev/sacralink/apps/server/internal/obs"
)

// SceneInfo descreve uma cena com as câmeras visíveis nela.
type SceneInfo struct {
	Name      string   `json:"name"`
	Program   bool     `json:"program"`
	Preview   bool     `json:"preview"`
	SacraLink bool     `json:"sacralink"` // cena dedicada criada pelo SacraLink
	CameraIDs []string `json:"cameraIds"`
}

// Scenes lista as cenas (ordem do OBS) e quais câmeras aparecem em cada uma.
func (b *Bridge) Scenes(ctx context.Context) ([]SceneInfo, error) {
	c := b.currentClient()
	if c == nil {
		return nil, ErrNotConnected
	}
	list, err := c.GetSceneList(ctx)
	if err != nil {
		return nil, err
	}
	sort.Slice(list.Scenes, func(i, j int) bool { return list.Scenes[i].SceneIndex > list.Scenes[j].SceneIndex })
	b.mu.Lock()
	defer b.mu.Unlock()
	ours := map[string]bool{}
	for _, m := range b.mappings {
		ours[m.SceneName] = true
	}
	out := make([]SceneInfo, 0, len(list.Scenes))
	for _, s := range list.Scenes {
		vis := b.graph.Visible(s.SceneName)
		info := SceneInfo{
			Name: s.SceneName, SacraLink: ours[s.SceneName],
			Program: s.SceneName == b.status.ProgramScene,
			Preview: b.status.StudioMode && s.SceneName == b.status.PreviewScene,
		}
		for id, m := range b.mappings {
			if vis[m.InputName] {
				info.CameraIDs = append(info.CameraIDs, id)
			}
		}
		sort.Strings(info.CameraIDs)
		out = append(out, info)
	}
	return out, nil
}

func (b *Bridge) cameraScene(cameraID string) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	m := b.mappings[cameraID]
	if m == nil || !b.present[cameraID] || m.SceneName == "" {
		return "", ErrNoSource
	}
	return m.SceneName, nil
}

// SetPreviewScene envia uma cena para o Preview (exige Studio Mode).
func (b *Bridge) SetPreviewScene(ctx context.Context, scene string) error {
	c := b.currentClient()
	if c == nil {
		return ErrNotConnected
	}
	if !b.Status().StudioMode {
		return ErrStudioModeOff
	}
	return c.SetCurrentPreviewScene(ctx, scene)
}

// SendCameraToPreview coloca a cena dedicada da câmera no Preview.
func (b *Bridge) SendCameraToPreview(ctx context.Context, cameraID string) error {
	scene, err := b.cameraScene(cameraID)
	if err != nil {
		return err
	}
	return b.SetPreviewScene(ctx, scene)
}

// Transition leva o Preview ao Program com a transição atual (Studio Mode).
func (b *Bridge) Transition(ctx context.Context) error {
	c := b.currentClient()
	if c == nil {
		return ErrNotConnected
	}
	if !b.Status().StudioMode {
		return ErrStudioModeOff
	}
	return c.TriggerStudioModeTransition(ctx)
}

// SetProgramScene coloca uma cena no ar. Com Studio Mode, passa pelo Preview e
// executa a transição; sem Studio Mode, exige confirmação explícita (ou a
// configuração de trocas diretas), pois a troca vai direto ao ar.
func (b *Bridge) SetProgramScene(ctx context.Context, scene string, confirmed bool) error {
	c := b.currentClient()
	if c == nil {
		return ErrNotConnected
	}
	if b.Status().StudioMode {
		if err := c.SetCurrentPreviewScene(ctx, scene); err != nil {
			return err
		}
		return c.TriggerStudioModeTransition(ctx)
	}
	if !confirmed && !b.opt.AllowDirectSwitch() {
		return ErrConfirmationRequired
	}
	return c.SetCurrentProgramScene(ctx, scene)
}

// SendCameraToProgram coloca a cena dedicada da câmera no ar.
func (b *Bridge) SendCameraToProgram(ctx context.Context, cameraID string, confirmed bool) error {
	scene, err := b.cameraScene(cameraID)
	if err != nil {
		return err
	}
	return b.SetProgramScene(ctx, scene, confirmed)
}

// ForceProgramScene troca o Program diretamente. Usado SOMENTE pela política
// de fallback automático habilitada explicitamente pelo operador.
func (b *Bridge) ForceProgramScene(ctx context.Context, scene string) error {
	c := b.currentClient()
	if c == nil {
		return ErrNotConnected
	}
	return c.SetCurrentProgramScene(ctx, scene)
}

// CameraSceneName retorna a cena dedicada de uma câmera.
func (b *Bridge) CameraSceneName(cameraID string) (string, error) { return b.cameraScene(cameraID) }

// Output identifica uma saída do OBS.
type Output string

// Saídas controláveis.
const (
	OutputStream Output = "stream"
	OutputRecord Output = "record"
)

// SetOutput inicia/para transmissão ou gravação. Exige confirmação sempre.
func (b *Bridge) SetOutput(ctx context.Context, out Output, start, confirmed bool) error {
	c := b.currentClient()
	if c == nil {
		return ErrNotConnected
	}
	if !confirmed {
		return ErrOutputConfirmRequired
	}
	switch {
	case out == OutputStream && start:
		return c.StartStream(ctx)
	case out == OutputStream:
		return c.StopStream(ctx)
	case out == OutputRecord && start:
		return c.StartRecord(ctx)
	default:
		return c.StopRecord(ctx)
	}
}

// OutputsInfo reúne estado e estatísticas das saídas.
type OutputsInfo struct {
	Stream      *obs.OutputStatus      `json:"stream"`
	Record      *obs.OutputStatus      `json:"record"`
	Stats       *obs.Stats             `json:"stats"`
	Destination *obs.StreamServiceInfo `json:"destination,omitempty"`
}

// Outputs consulta transmissão, gravação, estatísticas e destino (sem chave).
func (b *Bridge) Outputs(ctx context.Context) (*OutputsInfo, error) {
	c := b.currentClient()
	if c == nil {
		return nil, ErrNotConnected
	}
	info := &OutputsInfo{}
	var err error
	if info.Stream, err = c.GetStreamStatus(ctx); err != nil {
		return nil, err
	}
	if info.Record, err = c.GetRecordStatus(ctx); err != nil {
		return nil, err
	}
	if info.Stats, err = c.GetStats(ctx); err != nil {
		return nil, err
	}
	info.Destination, _ = c.GetStreamServiceInfo(ctx)
	return info, nil
}

// Transitions lista as transições disponíveis.
func (b *Bridge) Transitions(ctx context.Context) (*obs.TransitionList, error) {
	c := b.currentClient()
	if c == nil {
		return nil, ErrNotConnected
	}
	return c.GetSceneTransitionList(ctx)
}

// SetTransition define a transição atual.
func (b *Bridge) SetTransition(ctx context.Context, name string) error {
	c := b.currentClient()
	if c == nil {
		return ErrNotConnected
	}
	return c.SetCurrentSceneTransition(ctx, name)
}

// AudioInput descreve uma entrada de áudio do OBS.
type AudioInput struct {
	InputName string  `json:"inputName"`
	InputKind string  `json:"inputKind"`
	CameraID  string  `json:"cameraId,omitempty"`
	Muted     bool    `json:"muted"`
	VolumeDb  float64 `json:"volumeDb"`
}

// AudioInputs lista entradas que aceitam controle de volume (inclui as câmeras).
func (b *Bridge) AudioInputs(ctx context.Context) ([]AudioInput, error) {
	c := b.currentClient()
	if c == nil {
		return nil, ErrNotConnected
	}
	inputs, err := c.GetInputList(ctx)
	if err != nil {
		return nil, err
	}
	b.mu.Lock()
	byName := map[string]string{}
	for id, m := range b.mappings {
		byName[m.InputName] = id
	}
	b.mu.Unlock()
	out := []AudioInput{}
	for _, in := range inputs {
		vol, err := c.GetInputVolume(ctx, in.InputName)
		if err != nil {
			continue // entrada sem áudio
		}
		muted, _ := c.GetInputMute(ctx, in.InputName)
		out = append(out, AudioInput{InputName: in.InputName, InputKind: in.InputKind, CameraID: byName[in.InputName], Muted: muted, VolumeDb: vol.InputVolumeDb})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].InputName < out[j].InputName })
	return out, nil
}

// SetAudio ajusta mute, volume e/ou atraso de uma entrada do OBS.
func (b *Bridge) SetAudio(ctx context.Context, inputName string, muted *bool, volumeDb *float64, syncOffsetMs *int) error {
	c := b.currentClient()
	if c == nil {
		return ErrNotConnected
	}
	if muted != nil {
		if err := c.SetInputMute(ctx, inputName, *muted); err != nil {
			return err
		}
	}
	if volumeDb != nil {
		v := *volumeDb
		if v < -100 {
			v = -100
		}
		if v > 26 {
			v = 26
		}
		if err := c.SetInputVolumeDb(ctx, inputName, v); err != nil {
			return err
		}
	}
	if syncOffsetMs != nil {
		if err := c.SetInputAudioSyncOffset(ctx, inputName, *syncOffsetMs); err != nil {
			return err
		}
	}
	return nil
}

package obsbridge

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/gersongabrieldev/sacralink/apps/server/internal/obs"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/tally"
)

// fullSync lê todo o estado relevante do OBS e reconstrói o espelho local.
func (b *Bridge) fullSync(ctx context.Context, c *obs.Client) error {
	v, err := c.GetVersion(ctx)
	if err != nil {
		return fmt.Errorf("GetVersion: %w", err)
	}
	studio, err := c.GetStudioModeEnabled(ctx)
	if err != nil {
		return err
	}
	list, err := c.GetSceneList(ctx)
	if err != nil {
		return err
	}
	graph := tally.NewGraph()
	groups := map[string]bool{}
	for _, sc := range list.Scenes {
		if err := loadContainer(ctx, c, graph, groups, sc.SceneName, false); err != nil {
			return err
		}
	}
	stream, err := c.GetStreamStatus(ctx)
	if err != nil {
		return err
	}
	rec, err := c.GetRecordStatus(ctx)
	if err != nil {
		return err
	}
	video, err := c.GetVideoSettings(ctx)
	if err != nil {
		return err
	}
	inputs, err := c.GetInputList(ctx)
	if err != nil {
		return err
	}

	mappings, err := b.opt.Store.ListSourceMappings(ctx)
	if err != nil {
		return err
	}
	byUUID, byName := map[string]obs.Input{}, map[string]obs.Input{}
	for _, in := range inputs {
		if in.InputUUID != "" {
			byUUID[in.InputUUID] = in
		}
		byName[in.InputName] = in
	}
	present := map[string]bool{}
	mapIdx := map[string]*storeMapping{}
	for _, m := range mappings {
		found, ok := byUUID[m.InputUUID]
		if !ok || m.InputUUID == "" {
			found, ok = byName[m.InputName]
		}
		if ok {
			present[m.CameraID] = true
			if found.InputName != m.InputName {
				// Renomeada no OBS enquanto o SacraLink estava fora: seguimos o OBS.
				m.InputName = found.InputName
				_ = b.opt.Store.UpsertSourceMapping(ctx, m)
			}
		}
		mapIdx[m.CameraID] = m
	}
	active := map[string]bool{}
	for _, m := range mappings {
		if !present[m.CameraID] {
			continue
		}
		if a, err := c.GetSourceActive(ctx, m.InputName); err == nil && a.VideoActive {
			active[m.InputName] = true
		}
	}

	now := time.Now().UnixMilli()
	b.mu.Lock()
	b.graph, b.groups, b.active, b.present = graph, groups, active, present
	b.mappings = map[string]*storeMapping{}
	for id, m := range mapIdx {
		b.mappings[id] = m
	}
	b.status.OBSVersion, b.status.WSVersion = v.OBSVersion, v.OBSWebSocketVersion
	b.status.StudioMode = studio
	b.status.ProgramScene = list.CurrentProgramSceneName
	b.status.PreviewScene = ""
	if studio {
		b.status.PreviewScene = list.CurrentPreviewSceneName
	}
	b.status.Streaming, b.status.Recording = stream.OutputActive, rec.OutputActive
	b.status.StreamStarted = startedAt(stream)
	b.status.RecordStarted = startedAt(rec)
	b.status.CanvasWidth, b.status.CanvasHeight = video.BaseWidth, video.BaseHeight
	b.status.LastSyncAt = now
	b.mu.Unlock()

	if conn, err := b.opt.Store.GetOBSConnection(ctx); err == nil &&
		(conn.LastOBSVersion != v.OBSVersion || conn.LastWSVersion != v.OBSWebSocketVersion) {
		conn.LastOBSVersion, conn.LastWSVersion = v.OBSVersion, v.OBSWebSocketVersion
		_ = b.opt.Store.SaveOBSConnection(ctx, conn)
	}
	return nil
}

func startedAt(s *obs.OutputStatus) int64 {
	if !s.OutputActive {
		return 0
	}
	return time.Now().UnixMilli() - s.OutputDuration
}

// loadContainer carrega os itens de uma cena ou grupo e, recursivamente, dos
// grupos que ela contém (cenas aninhadas já são carregadas pela lista de cenas).
func loadContainer(ctx context.Context, c *obs.Client, g *tally.Graph, groups map[string]bool, name string, isGroup bool) error {
	var items []obs.SceneItem
	var err error
	if isGroup {
		items, err = c.GetGroupSceneItemList(ctx, name)
	} else {
		items, err = c.GetSceneItemList(ctx, name)
	}
	if err != nil {
		if obs.IsCode(err, obs.StatusResourceNotFound) {
			g.Remove(name)
			return nil
		}
		return fmt.Errorf("itens de %q: %w", name, err)
	}
	out := make([]tally.Item, 0, len(items))
	for _, it := range items {
		grp := it.IsGroup != nil && *it.IsGroup
		out = append(out, tally.Item{
			ID: it.SceneItemID, Source: it.SourceName, Enabled: it.SceneItemEnabled,
			Container: it.SourceType == obs.SourceTypeScene || grp,
		})
		if grp && !groups[it.SourceName] {
			groups[it.SourceName] = true
			if err := loadContainer(ctx, c, g, groups, it.SourceName, true); err != nil {
				return err
			}
		}
	}
	g.Set(name, out)
	return nil
}

// recomputeLocked recalcula o tally por câmera. Retorna o mapa calculado.
func (b *Bridge) recomputeLocked() map[string]tally.State {
	sources := make([]string, 0, len(b.mappings))
	byInput := map[string]string{}
	for id, m := range b.mappings {
		sources = append(sources, m.InputName)
		byInput[m.InputName] = id
	}
	res := tally.Compute(tally.Input{
		Connected:  b.status.State == StateConnected,
		Graph:      b.graph,
		Program:    b.status.ProgramScene,
		Preview:    b.status.PreviewScene,
		StudioMode: b.status.StudioMode,
		Active:     b.active,
	}, sources)
	out := make(map[string]tally.State, len(res))
	for name, st := range res {
		id := byInput[name]
		if !b.present[id] && st.Known {
			st = tally.State{Known: true} // fonte removida: não está no ar
		}
		out[id] = st
	}
	b.tallies = out
	return out
}

func (b *Bridge) emitTally() {
	b.mu.Lock()
	t := b.recomputeLocked()
	live := b.status.Streaming || b.status.Recording
	b.mu.Unlock()
	if b.opt.OnTally != nil {
		b.opt.OnTally(t, live)
	}
}

// scheduleResync agenda a releitura de uma cena/grupo com debounce de 80 ms.
func (b *Bridge) scheduleResync(ctx context.Context, c *obs.Client, name string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.pending[name] = true
	if b.pendingT != nil {
		return
	}
	b.pendingT = time.AfterFunc(80*time.Millisecond, func() {
		b.mu.Lock()
		names := b.pending
		b.pending = map[string]bool{}
		b.pendingT = nil
		groups := map[string]bool{}
		for k, v := range b.groups {
			groups[k] = v
		}
		b.mu.Unlock()
		g := tally.NewGraph()
		for n := range names {
			if err := loadContainer(ctx, c, g, groups, n, groups[n]); err != nil {
				b.log.Warn("falha ao ressincronizar cena", "cena", n, "erro", err)
			}
		}
		b.mu.Lock()
		for n := range names {
			if items, ok := g.Containers[n]; ok {
				b.graph.Set(n, items)
			} else {
				b.graph.Remove(n)
			}
		}
		for n, items := range g.Containers { // grupos descobertos
			b.graph.Set(n, items)
		}
		b.groups = groups
		b.mu.Unlock()
		b.emitTally()
	})
}

type eventScene struct {
	SceneName        string `json:"sceneName"`
	OldSceneName     string `json:"oldSceneName"`
	IsGroup          bool   `json:"isGroup"`
	SceneItemID      int    `json:"sceneItemId"`
	SceneItemEnabled bool   `json:"sceneItemEnabled"`
}

type eventInput struct {
	InputName    string `json:"inputName"`
	OldInputName string `json:"oldInputName"`
	InputUUID    string `json:"inputUuid"`
	VideoActive  bool   `json:"videoActive"`
}

type eventOutput struct {
	OutputActive bool   `json:"outputActive"`
	OutputState  string `json:"outputState"`
}

func (b *Bridge) handleEvent(ctx context.Context, c *obs.Client, ev obs.Event) {
	switch ev.Type {
	case obs.EventOverflow, "CurrentSceneCollectionChanged":
		if err := b.fullSync(ctx, c); err != nil {
			b.log.Warn("falha na ressincronização completa", "erro", err)
		}
		b.emitTally()
		b.notifyChange()

	case "CurrentProgramSceneChanged", "CurrentPreviewSceneChanged":
		var d eventScene
		_ = json.Unmarshal(ev.Data, &d)
		b.mu.Lock()
		if ev.Type == "CurrentProgramSceneChanged" {
			b.status.ProgramScene = d.SceneName
		} else if b.status.StudioMode {
			b.status.PreviewScene = d.SceneName
		}
		b.mu.Unlock()
		b.emitTally()
		b.notifyChange()

	case "StudioModeStateChanged":
		var d struct {
			StudioModeEnabled bool `json:"studioModeEnabled"`
		}
		_ = json.Unmarshal(ev.Data, &d)
		preview := ""
		if d.StudioModeEnabled {
			if l, err := c.GetSceneList(ctx); err == nil {
				preview = l.CurrentPreviewSceneName
			}
		}
		b.mu.Lock()
		b.status.StudioMode, b.status.PreviewScene = d.StudioModeEnabled, preview
		b.mu.Unlock()
		b.emitTally()
		b.notifyChange()

	case "SceneItemEnableStateChanged":
		var d eventScene
		_ = json.Unmarshal(ev.Data, &d)
		b.mu.Lock()
		ok := b.graph.SetItemEnabled(d.SceneName, d.SceneItemID, d.SceneItemEnabled)
		b.mu.Unlock()
		if ok {
			b.emitTally()
		} else {
			b.scheduleResync(ctx, c, d.SceneName)
		}

	case "SceneItemCreated", "SceneItemRemoved", "SceneItemListReindexed", "SceneCreated":
		var d eventScene
		_ = json.Unmarshal(ev.Data, &d)
		if ev.Type == "SceneCreated" && d.IsGroup {
			b.mu.Lock()
			b.groups[d.SceneName] = true
			b.mu.Unlock()
		}
		b.scheduleResync(ctx, c, d.SceneName)
		if ev.Type == "SceneCreated" {
			b.notifyChange()
		}

	case "SceneRemoved":
		var d eventScene
		_ = json.Unmarshal(ev.Data, &d)
		b.mu.Lock()
		b.graph.Remove(d.SceneName)
		b.mu.Unlock()
		b.emitTally()
		b.notifyChange()

	case "SceneNameChanged":
		var d eventScene
		_ = json.Unmarshal(ev.Data, &d)
		b.mu.Lock()
		b.graph.RenameSource(d.OldSceneName, d.SceneName)
		if b.status.ProgramScene == d.OldSceneName {
			b.status.ProgramScene = d.SceneName
		}
		if b.status.PreviewScene == d.OldSceneName {
			b.status.PreviewScene = d.SceneName
		}
		var changed []*storeMapping
		for _, m := range b.mappings {
			if m.SceneName == d.OldSceneName {
				m.SceneName = d.SceneName
				cp := *m // cópia: o banco escreve UpdatedAt fora do lock
				changed = append(changed, &cp)
			}
		}
		b.mu.Unlock()
		for _, m := range changed {
			_ = b.opt.Store.UpsertSourceMapping(ctx, m)
		}
		b.notifyChange()

	case "InputNameChanged":
		var d eventInput
		_ = json.Unmarshal(ev.Data, &d)
		b.mu.Lock()
		b.graph.RenameSource(d.OldInputName, d.InputName)
		if b.active[d.OldInputName] {
			delete(b.active, d.OldInputName)
			b.active[d.InputName] = true
		}
		var changed *storeMapping
		for _, m := range b.mappings {
			if m.InputName == d.OldInputName {
				m.InputName = d.InputName
				cp := *m // cópia: o banco escreve UpdatedAt fora do lock
				changed = &cp
			}
		}
		b.mu.Unlock()
		if changed != nil {
			_ = b.opt.Store.UpsertSourceMapping(ctx, changed)
			b.log.Info("fonte do SacraLink renomeada no OBS", "de", d.OldInputName, "para", d.InputName)
		}
		b.emitTally()
		b.notifyChange()

	case "InputCreated", "InputRemoved":
		var d eventInput
		_ = json.Unmarshal(ev.Data, &d)
		b.mu.Lock()
		for id, m := range b.mappings {
			if m.InputName == d.InputName || (d.InputUUID != "" && m.InputUUID == d.InputUUID) {
				b.present[id] = ev.Type == "InputCreated"
			}
		}
		if ev.Type == "InputRemoved" {
			delete(b.active, d.InputName)
		}
		b.mu.Unlock()
		b.emitTally()
		b.notifyChange()

	case "InputActiveStateChanged":
		var d eventInput
		_ = json.Unmarshal(ev.Data, &d)
		b.mu.Lock()
		if d.VideoActive {
			b.active[d.InputName] = true
		} else {
			delete(b.active, d.InputName)
		}
		b.mu.Unlock()
		b.emitTally()

	case "StreamStateChanged", "RecordStateChanged":
		var d eventOutput
		_ = json.Unmarshal(ev.Data, &d)
		b.mu.Lock()
		now := time.Now().UnixMilli()
		if ev.Type == "StreamStateChanged" {
			if d.OutputActive && !b.status.Streaming {
				b.status.StreamStarted = now
			}
			b.status.Streaming = d.OutputActive
		} else {
			if d.OutputActive && !b.status.Recording {
				b.status.RecordStarted = now
			}
			b.status.Recording = d.OutputActive
		}
		b.mu.Unlock()
		b.emitTally()
		b.notifyChange()

	case "InputVolumeMeters":
		b.handleMeters(ev.Data)
	}
}

// AudioLevel é o nível de uma entrada de áudio do OBS.
type AudioLevel struct {
	InputName string    `json:"inputName"`
	PeakDb    float64   `json:"peakDb"`
	MagDb     float64   `json:"magDb"`
	Channels  []float64 `json:"channelsDb"`
	Clipping  bool      `json:"clipping"`
}

func toDb(mul float64) float64 {
	if mul <= 0 {
		return -100
	}
	return math.Max(-100, 20*math.Log10(mul))
}

// handleMeters converte InputVolumeMeters (20 Hz) em níveis, limitado a 10 Hz.
func (b *Bridge) handleMeters(raw json.RawMessage) {
	if b.opt.OnAudio == nil {
		return
	}
	b.mu.Lock()
	if time.Since(b.lastAudio) < 100*time.Millisecond {
		b.mu.Unlock()
		return
	}
	b.lastAudio = time.Now()
	b.mu.Unlock()
	var d struct {
		Inputs []struct {
			InputName      string      `json:"inputName"`
			InputLevelsMul [][]float64 `json:"inputLevelsMul"`
		} `json:"inputs"`
	}
	if json.Unmarshal(raw, &d) != nil {
		return
	}
	out := make([]AudioLevel, 0, len(d.Inputs))
	for _, in := range d.Inputs {
		lv := AudioLevel{InputName: in.InputName, PeakDb: -100, MagDb: -100}
		for _, ch := range in.InputLevelsMul {
			if len(ch) < 2 {
				continue
			}
			mag, peak := ch[0], ch[1]
			lv.Channels = append(lv.Channels, toDb(peak))
			lv.PeakDb = math.Max(lv.PeakDb, toDb(peak))
			lv.MagDb = math.Max(lv.MagDb, toDb(mag))
			if peak >= 0.999 {
				lv.Clipping = true
			}
		}
		out = append(out, lv)
	}
	b.opt.OnAudio(out)
}

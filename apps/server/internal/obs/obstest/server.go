// Package obstest fornece um servidor obs-websocket 5.x simulado para testes
// automatizados. Ele implementa o handshake com autenticação, um modelo em
// memória de cenas/itens/grupos/entradas e emite os eventos correspondentes.
// NÃO substitui testes com o OBS real (ver docs/testing.md).
package obstest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"

	"github.com/coder/websocket"

	"github.com/gersongabrieldev/sacralink/apps/server/internal/obs"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/secure"
)

type item struct {
	id      int
	source  string
	enabled bool
}

type scene struct {
	name    string
	uuid    string
	isGroup bool
	items   []*item
}

// Input é uma entrada simulada.
type Input struct {
	Name     string
	UUID     string
	Kind     string
	Settings map[string]any
	Muted    bool
	VolumeDb float64
	OffsetMs int
}

// Server é um OBS simulado.
type Server struct {
	Password   string
	OBSVersion string
	WSVersion  string

	mu       sync.Mutex
	scenes   []*scene
	groups   map[string]*scene
	inputs   map[string]*Input
	program  string
	preview  string
	studio   bool
	stream   bool
	record   bool
	nextItem int
	calls    []string
	conns    map[*websocket.Conn]bool
	active   map[string]bool
	failNext map[string]int

	http *httptest.Server
}

// New cria e inicia o servidor simulado com uma cena "Cena" no Program.
func New(password string) *Server {
	s := &Server{
		Password: password, OBSVersion: "32.0.1", WSVersion: "5.6.3",
		groups: map[string]*scene{}, inputs: map[string]*Input{}, conns: map[*websocket.Conn]bool{},
		active: map[string]bool{}, failNext: map[string]int{},
	}
	s.scenes = []*scene{{name: "Cena", uuid: secure.NewID("u", 16)}}
	s.program = "Cena"
	s.http = httptest.NewServer(http.HandlerFunc(s.handle))
	return s
}

// URL retorna o endereço ws:// do servidor.
func (s *Server) URL() string { return "ws" + strings.TrimPrefix(s.http.URL, "http") }

// Close encerra o servidor e todas as conexões.
func (s *Server) Close() {
	s.mu.Lock()
	for c := range s.conns {
		c.CloseNow()
	}
	s.mu.Unlock()
	s.http.Close()
}

// DropConnections derruba as conexões (simula OBS fechando).
func (s *Server) DropConnections() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for c := range s.conns {
		c.CloseNow()
		delete(s.conns, c)
	}
}

// Calls retorna as requisições recebidas, em ordem.
func (s *Server) Calls() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.calls)
}

// FailNext faz as próximas n chamadas de requestType falharem.
func (s *Server) FailNext(requestType string, n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failNext[requestType] = n
}

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{Subprotocols: []string{obs.Subprotocol}})
	if err != nil {
		return
	}
	defer c.CloseNow()
	ctx := r.Context()
	hello := map[string]any{"obsWebSocketVersion": s.WSVersion, "rpcVersion": 1}
	challenge, salt := secure.NewToken(), secure.NewToken()
	if s.Password != "" {
		hello["authentication"] = map[string]string{"challenge": challenge, "salt": salt}
	}
	if s.write(ctx, c, obs.OpHello, hello) != nil {
		return
	}
	_, data, err := c.Read(ctx)
	if err != nil {
		return
	}
	var env struct {
		Op int `json:"op"`
		D  struct {
			RPCVersion     int    `json:"rpcVersion"`
			Authentication string `json:"authentication"`
		} `json:"d"`
	}
	if json.Unmarshal(data, &env) != nil || env.Op != obs.OpIdentify {
		c.Close(4002, "esperava Identify")
		return
	}
	if s.Password != "" && env.D.Authentication != obs.AuthString(s.Password, salt, challenge) {
		c.Close(obs.CloseAuthenticationFailed, "Authentication failed.")
		return
	}
	// Identified e inscrição nos eventos de forma atômica (os eventos são
	// emitidos com s.mu travado): nenhum evento chega antes do Identified e
	// nenhum evento disparado logo após a conexão se perde. Antes, a inscrição
	// vinha depois e um evento imediato sumia (teste intermitente no CI).
	s.mu.Lock()
	err = s.write(ctx, c, obs.OpIdentified, map[string]any{"negotiatedRpcVersion": 1})
	if err == nil {
		s.conns[c] = true
	}
	s.mu.Unlock()
	if err != nil {
		return
	}
	defer func() {
		s.mu.Lock()
		delete(s.conns, c)
		s.mu.Unlock()
	}()

	for {
		_, data, err := c.Read(ctx)
		if err != nil {
			return
		}
		var req struct {
			Op int `json:"op"`
			D  struct {
				RequestType string          `json:"requestType"`
				RequestID   string          `json:"requestId"`
				RequestData json.RawMessage `json:"requestData"`
			} `json:"d"`
		}
		if json.Unmarshal(data, &req) != nil || req.Op != obs.OpRequest {
			continue
		}
		resp, code, comment := s.dispatch(req.D.RequestType, req.D.RequestData)
		status := map[string]any{"result": code == obs.StatusSuccess, "code": code}
		if comment != "" {
			status["comment"] = comment
		}
		out := map[string]any{"requestType": req.D.RequestType, "requestId": req.D.RequestID, "requestStatus": status}
		if resp != nil {
			out["responseData"] = resp
		}
		if s.write(ctx, c, obs.OpRequestResponse, out) != nil {
			return
		}
	}
}

func (s *Server) write(ctx context.Context, c *websocket.Conn, op int, d any) error {
	b, _ := json.Marshal(map[string]any{"op": op, "d": d})
	return c.Write(ctx, websocket.MessageText, b)
}

// emitLocked envia um evento a todas as conexões (chamar com s.mu travado).
func (s *Server) emitLocked(eventType string, data map[string]any) {
	b, _ := json.Marshal(map[string]any{"op": obs.OpEvent, "d": map[string]any{"eventType": eventType, "eventIntent": 0, "eventData": data}})
	for c := range s.conns {
		_ = c.Write(context.Background(), websocket.MessageText, b)
	}
}

// ---------------------------------------------------------------------------
// Modelo

func (s *Server) sceneLocked(name string) *scene {
	for _, sc := range s.scenes {
		if sc.name == name {
			return sc
		}
	}
	return s.groups[name]
}

func (s *Server) isSceneLocked(name string) bool {
	for _, sc := range s.scenes {
		if sc.name == name {
			return true
		}
	}
	return false
}

func (s *Server) nameTakenLocked(name string) bool {
	_, isInput := s.inputs[name]
	return isInput || s.sceneLocked(name) != nil
}

// visibleLocked calcula as fontes visíveis a partir de uma cena (modelo simples).
func (s *Server) visibleLocked(root string, seen map[string]bool, out map[string]bool) {
	if seen[root] {
		return
	}
	seen[root] = true
	sc := s.sceneLocked(root)
	if sc == nil {
		return
	}
	for _, it := range sc.items {
		if !it.enabled {
			continue
		}
		out[it.source] = true
		s.visibleLocked(it.source, seen, out)
	}
}

// updateActiveLocked emite InputActiveStateChanged para entradas que mudaram.
func (s *Server) updateActiveLocked() {
	now := map[string]bool{}
	s.visibleLocked(s.program, map[string]bool{}, now)
	for name, in := range s.inputs {
		if now[name] != s.active[name] {
			s.emitLocked("InputActiveStateChanged", map[string]any{"inputName": name, "inputUuid": in.UUID, "videoActive": now[name]})
		}
	}
	s.active = map[string]bool{}
	for k, v := range now {
		if _, ok := s.inputs[k]; ok && v {
			s.active[k] = true
		}
	}
}

// AddScene cria uma cena.
func (s *Server) AddScene(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sc := &scene{name: name, uuid: secure.NewID("u", 16)}
	s.scenes = append(s.scenes, sc)
	s.emitLocked("SceneCreated", map[string]any{"sceneName": name, "sceneUuid": sc.uuid, "isGroup": false})
}

func (s *Server) addItemLocked(sceneName, source string, enabled bool) int {
	sc := s.sceneLocked(sceneName)
	if sc == nil {
		panic("cena inexistente: " + sceneName)
	}
	s.nextItem++
	sc.items = append(sc.items, &item{id: s.nextItem, source: source, enabled: enabled})
	s.emitLocked("SceneItemCreated", map[string]any{"sceneName": sceneName, "sourceName": source, "sceneItemId": s.nextItem, "sceneItemIndex": len(sc.items) - 1})
	s.updateActiveLocked()
	return s.nextItem
}

// AddInput cria uma entrada dentro de uma cena e retorna o sceneItemId.
func (s *Server) AddInput(sceneName, inputName, kind string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.inputs[inputName] = &Input{Name: inputName, UUID: secure.NewID("u", 16), Kind: kind, Settings: map[string]any{}}
	return s.addItemLocked(sceneName, inputName, true)
}

// AddSceneItem adiciona uma fonte existente (entrada, cena ou grupo) a uma cena.
func (s *Server) AddSceneItem(sceneName, source string, enabled bool) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.addItemLocked(sceneName, source, enabled)
}

// AddGroup cria um grupo dentro de uma cena contendo as fontes indicadas.
func (s *Server) AddGroup(sceneName, groupName string, members ...string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	g := &scene{name: groupName, uuid: secure.NewID("u", 16), isGroup: true}
	for _, m := range members {
		s.nextItem++
		g.items = append(g.items, &item{id: s.nextItem, source: m, enabled: true})
	}
	s.groups[groupName] = g
	s.addItemLocked(sceneName, groupName, true)
}

// SetItemEnabled altera a visibilidade de um item (como o "olho" do OBS).
func (s *Server) SetItemEnabled(sceneName, source string, enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sc := s.sceneLocked(sceneName)
	for _, it := range sc.items {
		if it.source == source {
			it.enabled = enabled
			s.emitLocked("SceneItemEnableStateChanged", map[string]any{"sceneName": sceneName, "sceneUuid": sc.uuid, "sceneItemId": it.id, "sceneItemEnabled": enabled})
		}
	}
	s.updateActiveLocked()
}

// SetProgram troca a cena do Program (como o operador clicando no OBS).
func (s *Server) SetProgram(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.setProgramLocked(name)
}

func (s *Server) setProgramLocked(name string) {
	s.program = name
	s.emitLocked("CurrentProgramSceneChanged", map[string]any{"sceneName": name, "sceneUuid": s.sceneLocked(name).uuid})
	s.updateActiveLocked()
}

// SetPreview troca a cena do Preview.
func (s *Server) SetPreview(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.preview = name
	s.emitLocked("CurrentPreviewSceneChanged", map[string]any{"sceneName": name, "sceneUuid": s.sceneLocked(name).uuid})
}

// SetStudioMode liga/desliga o Studio Mode.
func (s *Server) SetStudioMode(on bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.studio = on
	if on && s.preview == "" {
		s.preview = s.program
	}
	if !on {
		s.preview = ""
	}
	s.emitLocked("StudioModeStateChanged", map[string]any{"studioModeEnabled": on})
}

// SetStreaming simula início/fim da transmissão.
func (s *Server) SetStreaming(on bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stream = on
	state := "OBS_WEBSOCKET_OUTPUT_STOPPED"
	if on {
		state = "OBS_WEBSOCKET_OUTPUT_STARTED"
	}
	s.emitLocked("StreamStateChanged", map[string]any{"outputActive": on, "outputState": state})
}

// RemoveInputByUser remove uma entrada como se o usuário a tivesse apagado.
func (s *Server) RemoveInputByUser(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.removeInputLocked(name)
}

func (s *Server) removeInputLocked(name string) {
	in := s.inputs[name]
	if in == nil {
		return
	}
	for _, sc := range append(slices.Clone(s.scenes), mapsValues(s.groups)...) {
		sc.items = slices.DeleteFunc(sc.items, func(it *item) bool {
			if it.source == name {
				s.emitLocked("SceneItemRemoved", map[string]any{"sceneName": sc.name, "sourceName": name, "sceneItemId": it.id})
				return true
			}
			return false
		})
	}
	delete(s.inputs, name)
	delete(s.active, name)
	s.emitLocked("InputRemoved", map[string]any{"inputName": name, "inputUuid": in.UUID})
}

// RenameInputByUser renomeia uma entrada como o usuário faria no OBS.
func (s *Server) RenameInputByUser(oldName, newName string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.renameLocked(oldName, newName)
}

func (s *Server) renameLocked(oldName, newName string) {
	in := s.inputs[oldName]
	delete(s.inputs, oldName)
	in.Name = newName
	s.inputs[newName] = in
	for _, sc := range append(slices.Clone(s.scenes), mapsValues(s.groups)...) {
		for _, it := range sc.items {
			if it.source == oldName {
				it.source = newName
			}
		}
	}
	if s.active[oldName] {
		delete(s.active, oldName)
		s.active[newName] = true
	}
	s.emitLocked("InputNameChanged", map[string]any{"oldInputName": oldName, "inputName": newName, "inputUuid": in.UUID})
}

// Input retorna uma cópia da entrada (nil se não existir).
func (s *Server) Input(name string) *Input {
	s.mu.Lock()
	defer s.mu.Unlock()
	in := s.inputs[name]
	if in == nil {
		return nil
	}
	cp := *in
	return &cp
}

// HasScene informa se a cena existe.
func (s *Server) HasScene(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.isSceneLocked(name)
}

// Program retorna a cena atual do Program.
func (s *Server) Program() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.program
}

// Preview retorna a cena atual do Preview.
func (s *Server) Preview() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.preview
}

// SceneItems retorna as fontes de uma cena, na ordem.
func (s *Server) SceneItems(sceneName string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	if sc := s.sceneLocked(sceneName); sc != nil {
		for _, it := range sc.items {
			out = append(out, it.source)
		}
	}
	return out
}

func mapsValues(m map[string]*scene) []*scene {
	out := make([]*scene, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	return out
}

// ---------------------------------------------------------------------------
// Requisições

func (s *Server) dispatch(reqType string, raw json.RawMessage) (any, int, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, reqType)
	if n := s.failNext[reqType]; n > 0 {
		s.failNext[reqType] = n - 1
		return nil, 702, "falha simulada"
	}
	var p map[string]any
	_ = json.Unmarshal(raw, &p)
	str := func(k string) string { v, _ := p[k].(string); return v }
	notFound := func(what string) (any, int, string) {
		return nil, obs.StatusResourceNotFound, fmt.Sprintf("No source was found by the name of `%s`.", what)
	}

	switch reqType {
	case "GetVersion":
		return map[string]any{"obsVersion": s.OBSVersion, "obsWebSocketVersion": s.WSVersion, "rpcVersion": 1,
			"platform": "windows", "availableRequests": []string{}}, obs.StatusSuccess, ""
	case "GetSceneList":
		list := []map[string]any{}
		for i := len(s.scenes) - 1; i >= 0; i-- { // OBS lista em ordem reversa
			list = append(list, map[string]any{"sceneIndex": i, "sceneName": s.scenes[i].name, "sceneUuid": s.scenes[i].uuid})
		}
		return map[string]any{"currentProgramSceneName": s.program, "currentPreviewSceneName": nullable(s.preview), "scenes": list}, obs.StatusSuccess, ""
	case "GetSceneItemList", "GetGroupSceneItemList":
		sc := s.sceneLocked(str("sceneName"))
		if sc == nil || (reqType == "GetGroupSceneItemList") != sc.isGroup {
			return notFound(str("sceneName"))
		}
		items := []map[string]any{}
		for i, it := range sc.items {
			m := map[string]any{"sceneItemId": it.id, "sceneItemIndex": i, "sceneItemEnabled": it.enabled, "sourceName": it.source}
			if in, ok := s.inputs[it.source]; ok {
				m["sourceType"], m["inputKind"], m["isGroup"], m["sourceUuid"] = obs.SourceTypeInput, in.Kind, nil, in.UUID
			} else if g, ok := s.groups[it.source]; ok {
				m["sourceType"], m["inputKind"], m["isGroup"], m["sourceUuid"] = obs.SourceTypeScene, nil, true, g.uuid
			} else if child := s.sceneLocked(it.source); child != nil {
				m["sourceType"], m["inputKind"], m["isGroup"], m["sourceUuid"] = obs.SourceTypeScene, nil, false, child.uuid
			}
			items = append(items, m)
		}
		return map[string]any{"sceneItems": items}, obs.StatusSuccess, ""
	case "GetStudioModeEnabled":
		return map[string]any{"studioModeEnabled": s.studio}, obs.StatusSuccess, ""
	case "SetStudioModeEnabled":
		s.studio, _ = p["studioModeEnabled"].(bool)
		return nil, obs.StatusSuccess, ""
	case "GetCurrentProgramScene":
		return map[string]any{"currentProgramSceneName": s.program, "sceneName": s.program}, obs.StatusSuccess, ""
	case "GetCurrentPreviewScene":
		if !s.studio {
			return nil, obs.StatusStudioModeNotActive, "Studio mode is not active."
		}
		return map[string]any{"currentPreviewSceneName": s.preview, "sceneName": s.preview}, obs.StatusSuccess, ""
	case "SetCurrentProgramScene":
		if !s.isSceneLocked(str("sceneName")) {
			return notFound(str("sceneName"))
		}
		s.setProgramLocked(str("sceneName"))
		return nil, obs.StatusSuccess, ""
	case "SetCurrentPreviewScene":
		if !s.studio {
			return nil, obs.StatusStudioModeNotActive, "Studio mode is not active."
		}
		if !s.isSceneLocked(str("sceneName")) {
			return notFound(str("sceneName"))
		}
		s.preview = str("sceneName")
		s.emitLocked("CurrentPreviewSceneChanged", map[string]any{"sceneName": s.preview})
		return nil, obs.StatusSuccess, ""
	case "TriggerStudioModeTransition":
		if !s.studio {
			return nil, obs.StatusStudioModeNotActive, "Studio mode is not active."
		}
		old := s.program
		s.setProgramLocked(s.preview)
		s.preview = old // comportamento padrão do OBS (swap)
		s.emitLocked("CurrentPreviewSceneChanged", map[string]any{"sceneName": s.preview})
		return nil, obs.StatusSuccess, ""
	case "GetInputList":
		list := []map[string]any{}
		for _, in := range s.inputs {
			list = append(list, map[string]any{"inputName": in.Name, "inputUuid": in.UUID, "inputKind": in.Kind, "unversionedInputKind": in.Kind})
		}
		return map[string]any{"inputs": list}, obs.StatusSuccess, ""
	case "GetInputSettings":
		in := s.inputs[str("inputName")]
		if in == nil {
			return notFound(str("inputName"))
		}
		return map[string]any{"inputSettings": in.Settings, "inputKind": in.Kind}, obs.StatusSuccess, ""
	case "SetInputSettings":
		in := s.inputs[str("inputName")]
		if in == nil {
			return notFound(str("inputName"))
		}
		settings, _ := p["inputSettings"].(map[string]any)
		if overlay, _ := p["overlay"].(bool); !overlay {
			in.Settings = map[string]any{}
		}
		for k, v := range settings {
			in.Settings[k] = v
		}
		return nil, obs.StatusSuccess, ""
	case "CreateScene":
		name := str("sceneName")
		if s.nameTakenLocked(name) {
			return nil, obs.StatusResourceAlreadyExist, "A source already exists by that scene name."
		}
		sc := &scene{name: name, uuid: secure.NewID("u", 16)}
		s.scenes = append(s.scenes, sc)
		s.emitLocked("SceneCreated", map[string]any{"sceneName": name, "sceneUuid": sc.uuid, "isGroup": false})
		return map[string]any{"sceneUuid": sc.uuid}, obs.StatusSuccess, ""
	case "CreateInput":
		name, sceneName := str("inputName"), str("sceneName")
		if s.nameTakenLocked(name) {
			return nil, obs.StatusResourceAlreadyExist, "A source already exists by that input name."
		}
		if s.sceneLocked(sceneName) == nil {
			return notFound(sceneName)
		}
		settings, _ := p["inputSettings"].(map[string]any)
		if settings == nil {
			settings = map[string]any{}
		}
		in := &Input{Name: name, UUID: secure.NewID("u", 16), Kind: str("inputKind"), Settings: settings}
		s.inputs[name] = in
		s.emitLocked("InputCreated", map[string]any{"inputName": name, "inputUuid": in.UUID, "inputKind": in.Kind})
		enabled := true
		if v, ok := p["sceneItemEnabled"].(bool); ok {
			enabled = v
		}
		id := s.addItemLocked(sceneName, name, enabled)
		return map[string]any{"inputUuid": in.UUID, "sceneItemId": id}, obs.StatusSuccess, ""
	case "CreateSceneItem":
		if s.sceneLocked(str("sceneName")) == nil {
			return notFound(str("sceneName"))
		}
		if _, ok := s.inputs[str("sourceName")]; !ok && s.sceneLocked(str("sourceName")) == nil {
			return notFound(str("sourceName"))
		}
		enabled := true
		if v, ok := p["sceneItemEnabled"].(bool); ok {
			enabled = v
		}
		id := s.addItemLocked(str("sceneName"), str("sourceName"), enabled)
		return map[string]any{"sceneItemId": id}, obs.StatusSuccess, ""
	case "RemoveInput":
		if s.inputs[str("inputName")] == nil {
			return notFound(str("inputName"))
		}
		s.removeInputLocked(str("inputName"))
		s.updateActiveLocked()
		return nil, obs.StatusSuccess, ""
	case "SetInputName":
		if s.inputs[str("inputName")] == nil {
			return notFound(str("inputName"))
		}
		if s.nameTakenLocked(str("newInputName")) {
			return nil, obs.StatusResourceAlreadyExist, "name taken"
		}
		s.renameLocked(str("inputName"), str("newInputName"))
		return nil, obs.StatusSuccess, ""
	case "GetVideoSettings":
		return map[string]any{"baseWidth": 1920, "baseHeight": 1080, "outputWidth": 1920, "outputHeight": 1080, "fpsNumerator": 30, "fpsDenominator": 1}, obs.StatusSuccess, ""
	case "SetSceneItemTransform":
		return nil, obs.StatusSuccess, ""
	case "GetSourceActive":
		name := str("sourceName")
		if _, ok := s.inputs[name]; !ok && s.sceneLocked(name) == nil {
			return notFound(name)
		}
		vis := map[string]bool{}
		s.visibleLocked(s.program, map[string]bool{}, vis)
		return map[string]any{"videoActive": vis[name], "videoShowing": vis[name]}, obs.StatusSuccess, ""
	case "GetStreamStatus":
		return map[string]any{"outputActive": s.stream, "outputTimecode": "00:00:00.000", "outputDuration": 0, "outputBytes": 0}, obs.StatusSuccess, ""
	case "GetRecordStatus":
		return map[string]any{"outputActive": s.record, "outputTimecode": "00:00:00.000", "outputDuration": 0, "outputBytes": 0}, obs.StatusSuccess, ""
	case "StartStream":
		if s.stream {
			return nil, obs.StatusOutputRunning, ""
		}
		s.stream = true
		s.emitLocked("StreamStateChanged", map[string]any{"outputActive": true, "outputState": "OBS_WEBSOCKET_OUTPUT_STARTED"})
		return nil, obs.StatusSuccess, ""
	case "StopStream":
		if !s.stream {
			return nil, obs.StatusOutputNotRunning, ""
		}
		s.stream = false
		s.emitLocked("StreamStateChanged", map[string]any{"outputActive": false, "outputState": "OBS_WEBSOCKET_OUTPUT_STOPPED"})
		return nil, obs.StatusSuccess, ""
	case "StartRecord":
		s.record = true
		s.emitLocked("RecordStateChanged", map[string]any{"outputActive": true, "outputState": "OBS_WEBSOCKET_OUTPUT_STARTED"})
		return nil, obs.StatusSuccess, ""
	case "StopRecord":
		s.record = false
		s.emitLocked("RecordStateChanged", map[string]any{"outputActive": false, "outputState": "OBS_WEBSOCKET_OUTPUT_STOPPED"})
		return nil, obs.StatusSuccess, ""
	case "GetStats":
		return map[string]any{"cpuUsage": 3.5, "memoryUsage": 512.0, "activeFps": 30.0, "renderSkippedFrames": 0, "renderTotalFrames": 1000}, obs.StatusSuccess, ""
	case "GetStreamServiceSettings":
		return map[string]any{"streamServiceType": "rtmp_common", "streamServiceSettings": map[string]any{"service": "YouTube - RTMPS", "server": "auto", "key": "NAO-DEVE-VAZAR"}}, obs.StatusSuccess, ""
	case "GetSceneTransitionList":
		return map[string]any{"currentSceneTransitionName": "Fade", "currentSceneTransitionKind": "fade_transition",
			"transitions": []map[string]any{{"transitionName": "Fade", "transitionKind": "fade_transition"}, {"transitionName": "Cut", "transitionKind": "cut_transition", "transitionFixed": true}}}, obs.StatusSuccess, ""
	case "SetCurrentSceneTransition":
		return nil, obs.StatusSuccess, ""
	case "GetInputMute", "SetInputMute", "GetInputVolume", "SetInputVolume", "SetInputAudioSyncOffset":
		in := s.inputs[str("inputName")]
		if in == nil {
			return notFound(str("inputName"))
		}
		switch reqType {
		case "GetInputMute":
			return map[string]any{"inputMuted": in.Muted}, obs.StatusSuccess, ""
		case "SetInputMute":
			in.Muted, _ = p["inputMuted"].(bool)
			s.emitLocked("InputMuteStateChanged", map[string]any{"inputName": in.Name, "inputMuted": in.Muted})
		case "GetInputVolume":
			return map[string]any{"inputVolumeDb": in.VolumeDb, "inputVolumeMul": 1.0}, obs.StatusSuccess, ""
		case "SetInputVolume":
			in.VolumeDb, _ = p["inputVolumeDb"].(float64)
		case "SetInputAudioSyncOffset":
			v, _ := p["inputAudioSyncOffset"].(float64)
			in.OffsetMs = int(v)
		}
		return nil, obs.StatusSuccess, ""
	case "GetSpecialInputs":
		return map[string]any{"desktop1": nil, "mic1": nil}, obs.StatusSuccess, ""
	}
	return nil, 204, "Your request type is not valid."
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

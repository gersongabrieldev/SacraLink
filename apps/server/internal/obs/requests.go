package obs

import (
	"context"
	"encoding/json"
)

// Wrappers tipados das requisições usadas pelo SacraLink. Os nomes de campo
// seguem exatamente o protocolo obs-websocket 5.x.

// Version é a resposta de GetVersion.
type Version struct {
	OBSVersion          string   `json:"obsVersion"`
	OBSWebSocketVersion string   `json:"obsWebSocketVersion"`
	RPCVersion          int      `json:"rpcVersion"`
	AvailableRequests   []string `json:"availableRequests"`
	Platform            string   `json:"platform"`
	PlatformDescription string   `json:"platformDescription"`
}

// GetVersion consulta as versões do OBS e do obs-websocket.
func (c *Client) GetVersion(ctx context.Context) (*Version, error) {
	v := &Version{}
	return v, c.Call(ctx, "GetVersion", nil, v)
}

// Scene é um item de GetSceneList.
type Scene struct {
	SceneIndex int    `json:"sceneIndex"`
	SceneName  string `json:"sceneName"`
	SceneUUID  string `json:"sceneUuid,omitempty"`
}

// SceneList é a resposta de GetSceneList.
type SceneList struct {
	CurrentProgramSceneName string  `json:"currentProgramSceneName"`
	CurrentProgramSceneUUID string  `json:"currentProgramSceneUuid,omitempty"`
	CurrentPreviewSceneName string  `json:"currentPreviewSceneName"`
	CurrentPreviewSceneUUID string  `json:"currentPreviewSceneUuid,omitempty"`
	Scenes                  []Scene `json:"scenes"`
}

// GetSceneList lista as cenas da coleção atual.
func (c *Client) GetSceneList(ctx context.Context) (*SceneList, error) {
	l := &SceneList{}
	return l, c.Call(ctx, "GetSceneList", nil, l)
}

// SceneItem é um item de GetSceneItemList / GetGroupSceneItemList.
type SceneItem struct {
	InputKind        string          `json:"inputKind,omitempty"`
	IsGroup          *bool           `json:"isGroup"`
	SceneItemEnabled bool            `json:"sceneItemEnabled"`
	SceneItemID      int             `json:"sceneItemId"`
	SceneItemIndex   int             `json:"sceneItemIndex"`
	SourceName       string          `json:"sourceName"`
	SourceType       string          `json:"sourceType"`
	SourceUUID       string          `json:"sourceUuid,omitempty"`
	Transform        json.RawMessage `json:"sceneItemTransform,omitempty"`
}

// Tipos de fonte relevantes.
const (
	SourceTypeInput = "OBS_SOURCE_TYPE_INPUT"
	SourceTypeScene = "OBS_SOURCE_TYPE_SCENE"
)

// GetSceneItemList lista os itens de uma cena.
func (c *Client) GetSceneItemList(ctx context.Context, sceneName string) ([]SceneItem, error) {
	var r struct {
		SceneItems []SceneItem `json:"sceneItems"`
	}
	err := c.Call(ctx, "GetSceneItemList", map[string]any{"sceneName": sceneName}, &r)
	return r.SceneItems, err
}

// GetGroupSceneItemList lista os itens de um grupo.
func (c *Client) GetGroupSceneItemList(ctx context.Context, groupName string) ([]SceneItem, error) {
	var r struct {
		SceneItems []SceneItem `json:"sceneItems"`
	}
	err := c.Call(ctx, "GetGroupSceneItemList", map[string]any{"sceneName": groupName}, &r)
	return r.SceneItems, err
}

// GetStudioModeEnabled informa se o Studio Mode está ativo.
func (c *Client) GetStudioModeEnabled(ctx context.Context) (bool, error) {
	var r struct {
		StudioModeEnabled bool `json:"studioModeEnabled"`
	}
	err := c.Call(ctx, "GetStudioModeEnabled", nil, &r)
	return r.StudioModeEnabled, err
}

// SetCurrentProgramScene troca a cena do Program (vai ao ar).
func (c *Client) SetCurrentProgramScene(ctx context.Context, sceneName string) error {
	return c.Call(ctx, "SetCurrentProgramScene", map[string]any{"sceneName": sceneName}, nil)
}

// SetCurrentPreviewScene troca a cena do Preview (requer Studio Mode).
func (c *Client) SetCurrentPreviewScene(ctx context.Context, sceneName string) error {
	return c.Call(ctx, "SetCurrentPreviewScene", map[string]any{"sceneName": sceneName}, nil)
}

// TriggerStudioModeTransition leva o Preview ao Program com a transição atual.
func (c *Client) TriggerStudioModeTransition(ctx context.Context) error {
	return c.Call(ctx, "TriggerStudioModeTransition", nil, nil)
}

// Input é um item de GetInputList.
type Input struct {
	InputKind            string `json:"inputKind"`
	InputName            string `json:"inputName"`
	InputUUID            string `json:"inputUuid,omitempty"`
	UnversionedInputKind string `json:"unversionedInputKind"`
}

// GetInputList lista as entradas (fontes que não são cenas).
func (c *Client) GetInputList(ctx context.Context) ([]Input, error) {
	var r struct {
		Inputs []Input `json:"inputs"`
	}
	err := c.Call(ctx, "GetInputList", nil, &r)
	return r.Inputs, err
}

// GetInputSettings retorna as configurações de uma entrada.
func (c *Client) GetInputSettings(ctx context.Context, inputName string) (map[string]any, string, error) {
	var r struct {
		InputSettings map[string]any `json:"inputSettings"`
		InputKind     string         `json:"inputKind"`
	}
	err := c.Call(ctx, "GetInputSettings", map[string]any{"inputName": inputName}, &r)
	return r.InputSettings, r.InputKind, err
}

// SetInputSettings aplica configurações (overlay=true mescla com as atuais).
func (c *Client) SetInputSettings(ctx context.Context, inputName string, settings map[string]any, overlay bool) error {
	return c.Call(ctx, "SetInputSettings", map[string]any{"inputName": inputName, "inputSettings": settings, "overlay": overlay}, nil)
}

// CreateScene cria uma cena e retorna seu UUID (vazio em obs-websocket < 5.3).
func (c *Client) CreateScene(ctx context.Context, sceneName string) (string, error) {
	var r struct {
		SceneUUID string `json:"sceneUuid"`
	}
	err := c.Call(ctx, "CreateScene", map[string]any{"sceneName": sceneName}, &r)
	return r.SceneUUID, err
}

// CreatedInput é a resposta de CreateInput.
type CreatedInput struct {
	InputUUID   string `json:"inputUuid"`
	SceneItemID int    `json:"sceneItemId"`
}

// CreateInput cria uma entrada dentro de uma cena.
func (c *Client) CreateInput(ctx context.Context, sceneName, inputName, kind string, settings map[string]any, enabled bool) (*CreatedInput, error) {
	r := &CreatedInput{}
	err := c.Call(ctx, "CreateInput", map[string]any{
		"sceneName": sceneName, "inputName": inputName, "inputKind": kind,
		"inputSettings": settings, "sceneItemEnabled": enabled,
	}, r)
	return r, err
}

// CreateSceneItem adiciona uma fonte existente a uma cena.
func (c *Client) CreateSceneItem(ctx context.Context, sceneName, sourceName string, enabled bool) (int, error) {
	var r struct {
		SceneItemID int `json:"sceneItemId"`
	}
	err := c.Call(ctx, "CreateSceneItem", map[string]any{"sceneName": sceneName, "sourceName": sourceName, "sceneItemEnabled": enabled}, &r)
	return r.SceneItemID, err
}

// RemoveInput remove uma entrada (usado apenas para entradas do SacraLink).
func (c *Client) RemoveInput(ctx context.Context, inputName string) error {
	return c.Call(ctx, "RemoveInput", map[string]any{"inputName": inputName}, nil)
}

// SetInputName renomeia uma entrada.
func (c *Client) SetInputName(ctx context.Context, inputName, newName string) error {
	return c.Call(ctx, "SetInputName", map[string]any{"inputName": inputName, "newInputName": newName}, nil)
}

// SetSceneName renomeia uma cena.
func (c *Client) SetSceneName(ctx context.Context, sceneName, newName string) error {
	return c.Call(ctx, "SetSceneName", map[string]any{"sceneName": sceneName, "newSceneName": newName}, nil)
}

// VideoSettings é a resposta de GetVideoSettings.
type VideoSettings struct {
	BaseWidth      int `json:"baseWidth"`
	BaseHeight     int `json:"baseHeight"`
	OutputWidth    int `json:"outputWidth"`
	OutputHeight   int `json:"outputHeight"`
	FPSNumerator   int `json:"fpsNumerator"`
	FPSDenominator int `json:"fpsDenominator"`
}

// GetVideoSettings retorna a resolução de produção (canvas).
func (c *Client) GetVideoSettings(ctx context.Context) (*VideoSettings, error) {
	v := &VideoSettings{}
	return v, c.Call(ctx, "GetVideoSettings", nil, v)
}

// SetSceneItemTransform aplica transformação a um item de cena.
func (c *Client) SetSceneItemTransform(ctx context.Context, sceneName string, itemID int, transform map[string]any) error {
	return c.Call(ctx, "SetSceneItemTransform", map[string]any{
		"sceneName": sceneName, "sceneItemId": itemID, "sceneItemTransform": transform,
	}, nil)
}

// SourceActive é a resposta de GetSourceActive.
type SourceActive struct {
	VideoActive  bool `json:"videoActive"`
	VideoShowing bool `json:"videoShowing"`
}

// GetSourceActive consulta se a fonte está ativa no Program (libobs).
func (c *Client) GetSourceActive(ctx context.Context, sourceName string) (*SourceActive, error) {
	r := &SourceActive{}
	return r, c.Call(ctx, "GetSourceActive", map[string]any{"sourceName": sourceName}, r)
}

// OutputStatus é a resposta de GetStreamStatus / GetRecordStatus.
type OutputStatus struct {
	OutputActive        bool    `json:"outputActive"`
	OutputPaused        bool    `json:"outputPaused,omitempty"`
	OutputReconnecting  bool    `json:"outputReconnecting,omitempty"`
	OutputTimecode      string  `json:"outputTimecode"`
	OutputDuration      int64   `json:"outputDuration"`
	OutputCongestion    float64 `json:"outputCongestion,omitempty"`
	OutputBytes         int64   `json:"outputBytes"`
	OutputSkippedFrames int64   `json:"outputSkippedFrames,omitempty"`
	OutputTotalFrames   int64   `json:"outputTotalFrames,omitempty"`
}

// GetStreamStatus retorna o estado da transmissão.
func (c *Client) GetStreamStatus(ctx context.Context) (*OutputStatus, error) {
	s := &OutputStatus{}
	return s, c.Call(ctx, "GetStreamStatus", nil, s)
}

// GetRecordStatus retorna o estado da gravação.
func (c *Client) GetRecordStatus(ctx context.Context) (*OutputStatus, error) {
	s := &OutputStatus{}
	return s, c.Call(ctx, "GetRecordStatus", nil, s)
}

// Stats é a resposta de GetStats.
type Stats struct {
	CPUUsage                 float64 `json:"cpuUsage"`
	MemoryUsage              float64 `json:"memoryUsage"`
	AvailableDiskSpace       float64 `json:"availableDiskSpace"`
	ActiveFPS                float64 `json:"activeFps"`
	AverageFrameRenderTime   float64 `json:"averageFrameRenderTime"`
	RenderSkippedFrames      int64   `json:"renderSkippedFrames"`
	RenderTotalFrames        int64   `json:"renderTotalFrames"`
	OutputSkippedFrames      int64   `json:"outputSkippedFrames"`
	OutputTotalFrames        int64   `json:"outputTotalFrames"`
	WebSocketSessionIncoming int64   `json:"webSocketSessionIncomingMessages"`
	WebSocketSessionOutgoing int64   `json:"webSocketSessionOutgoingMessages"`
}

// GetStats retorna estatísticas de desempenho do OBS.
func (c *Client) GetStats(ctx context.Context) (*Stats, error) {
	s := &Stats{}
	return s, c.Call(ctx, "GetStats", nil, s)
}

// StreamServiceInfo resume o destino da transmissão SEM a chave de stream.
type StreamServiceInfo struct {
	Type    string `json:"type"`
	Service string `json:"service,omitempty"`
	Server  string `json:"server,omitempty"`
}

// GetStreamServiceInfo retorna o destino configurado, descartando a chave.
func (c *Client) GetStreamServiceInfo(ctx context.Context) (*StreamServiceInfo, error) {
	var r struct {
		StreamServiceType     string         `json:"streamServiceType"`
		StreamServiceSettings map[string]any `json:"streamServiceSettings"`
	}
	if err := c.Call(ctx, "GetStreamServiceSettings", nil, &r); err != nil {
		return nil, err
	}
	info := &StreamServiceInfo{Type: r.StreamServiceType}
	if s, ok := r.StreamServiceSettings["service"].(string); ok {
		info.Service = s
	}
	if s, ok := r.StreamServiceSettings["server"].(string); ok {
		info.Server = s
	}
	return info, nil
}

// StartStream inicia a transmissão.
func (c *Client) StartStream(ctx context.Context) error { return c.Call(ctx, "StartStream", nil, nil) }

// StopStream encerra a transmissão.
func (c *Client) StopStream(ctx context.Context) error { return c.Call(ctx, "StopStream", nil, nil) }

// StartRecord inicia a gravação.
func (c *Client) StartRecord(ctx context.Context) error { return c.Call(ctx, "StartRecord", nil, nil) }

// StopRecord encerra a gravação.
func (c *Client) StopRecord(ctx context.Context) error { return c.Call(ctx, "StopRecord", nil, nil) }

// Transition é um item de GetSceneTransitionList.
type Transition struct {
	TransitionName         string `json:"transitionName"`
	TransitionKind         string `json:"transitionKind"`
	TransitionFixed        bool   `json:"transitionFixed"`
	TransitionConfigurable bool   `json:"transitionConfigurable"`
}

// TransitionList é a resposta de GetSceneTransitionList.
type TransitionList struct {
	CurrentSceneTransitionName string       `json:"currentSceneTransitionName"`
	CurrentSceneTransitionKind string       `json:"currentSceneTransitionKind"`
	Transitions                []Transition `json:"transitions"`
}

// GetSceneTransitionList lista as transições.
func (c *Client) GetSceneTransitionList(ctx context.Context) (*TransitionList, error) {
	l := &TransitionList{}
	return l, c.Call(ctx, "GetSceneTransitionList", nil, l)
}

// SetCurrentSceneTransition define a transição atual.
func (c *Client) SetCurrentSceneTransition(ctx context.Context, name string) error {
	return c.Call(ctx, "SetCurrentSceneTransition", map[string]any{"transitionName": name}, nil)
}

// SetInputMute muta/desmuta uma entrada de áudio.
func (c *Client) SetInputMute(ctx context.Context, inputName string, muted bool) error {
	return c.Call(ctx, "SetInputMute", map[string]any{"inputName": inputName, "inputMuted": muted}, nil)
}

// GetInputMute consulta o mute de uma entrada.
func (c *Client) GetInputMute(ctx context.Context, inputName string) (bool, error) {
	var r struct {
		InputMuted bool `json:"inputMuted"`
	}
	err := c.Call(ctx, "GetInputMute", map[string]any{"inputName": inputName}, &r)
	return r.InputMuted, err
}

// InputVolume é a resposta de GetInputVolume.
type InputVolume struct {
	InputVolumeMul float64 `json:"inputVolumeMul"`
	InputVolumeDb  float64 `json:"inputVolumeDb"`
}

// GetInputVolume consulta o volume de uma entrada.
func (c *Client) GetInputVolume(ctx context.Context, inputName string) (*InputVolume, error) {
	v := &InputVolume{}
	return v, c.Call(ctx, "GetInputVolume", map[string]any{"inputName": inputName}, v)
}

// SetInputVolumeDb ajusta o volume em dB (-100 a 26).
func (c *Client) SetInputVolumeDb(ctx context.Context, inputName string, db float64) error {
	return c.Call(ctx, "SetInputVolume", map[string]any{"inputName": inputName, "inputVolumeDb": db}, nil)
}

// SetInputAudioSyncOffset ajusta o atraso de áudio em ms (-950 a 20000).
func (c *Client) SetInputAudioSyncOffset(ctx context.Context, inputName string, offsetMs int) error {
	return c.Call(ctx, "SetInputAudioSyncOffset", map[string]any{"inputName": inputName, "inputAudioSyncOffset": offsetMs}, nil)
}

// SpecialInputs é a resposta de GetSpecialInputs.
type SpecialInputs struct {
	Desktop1 string `json:"desktop1"`
	Desktop2 string `json:"desktop2"`
	Mic1     string `json:"mic1"`
	Mic2     string `json:"mic2"`
	Mic3     string `json:"mic3"`
	Mic4     string `json:"mic4"`
}

// GetSpecialInputs retorna as entradas globais de áudio.
func (c *Client) GetSpecialInputs(ctx context.Context) (*SpecialInputs, error) {
	s := &SpecialInputs{}
	return s, c.Call(ctx, "GetSpecialInputs", nil, s)
}

// SceneCollections é a resposta de GetSceneCollectionList.
type SceneCollections struct {
	Current     string   `json:"currentSceneCollectionName"`
	Collections []string `json:"sceneCollections"`
}

// GetSceneCollectionList lista as coleções de cenas.
func (c *Client) GetSceneCollectionList(ctx context.Context) (*SceneCollections, error) {
	s := &SceneCollections{}
	return s, c.Call(ctx, "GetSceneCollectionList", nil, s)
}

// CreateSceneCollection cria uma coleção e muda o OBS para ela.
func (c *Client) CreateSceneCollection(ctx context.Context, name string) error {
	return c.Call(ctx, "CreateSceneCollection", map[string]any{"sceneCollectionName": name}, nil)
}

// SetCurrentSceneCollection muda a coleção de cenas atual.
func (c *Client) SetCurrentSceneCollection(ctx context.Context, name string) error {
	return c.Call(ctx, "SetCurrentSceneCollection", map[string]any{"sceneCollectionName": name}, nil)
}

// GetSourceScreenshot captura a imagem atual de uma fonte (data URI base64).
// Prova que o OBS está de fato decodificando o vídeo da câmera.
func (c *Client) GetSourceScreenshot(ctx context.Context, sourceName, format string, width int) (string, error) {
	var r struct {
		ImageData string `json:"imageData"`
	}
	err := c.Call(ctx, "GetSourceScreenshot", map[string]any{"sourceName": sourceName, "imageFormat": format, "imageWidth": width}, &r)
	return r.ImageData, err
}

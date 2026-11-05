// Package obs implementa um cliente do protocolo obs-websocket 5.x
// (https://github.com/obsproject/obs-websocket/blob/master/docs/generated/protocol.md).
//
// O cliente é deliberadamente pequeno: handshake com autenticação, chamadas
// de requisição/resposta correlacionadas por requestId e entrega de eventos.
// A lógica do SacraLink (tally, fontes, cenas) fica no pacote obsbridge.
package obs

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
)

// Subprotocolo WebSocket do obs-websocket com codificação JSON.
const Subprotocol = "obswebsocket.json"

// RPCVersion suportada.
const RPCVersion = 1

// OpCodes do protocolo.
const (
	OpHello             = 0
	OpIdentify          = 1
	OpIdentified        = 2
	OpReidentify        = 3
	OpEvent             = 5
	OpRequest           = 6
	OpRequestResponse   = 7
	OpRequestBatch      = 8
	OpRequestBatchReply = 9
)

// EventSubscription é a máscara de assinaturas de eventos.
type EventSubscription uint32

// Categorias de eventos.
const (
	SubGeneral     EventSubscription = 1 << 0
	SubConfig      EventSubscription = 1 << 1
	SubScenes      EventSubscription = 1 << 2
	SubInputs      EventSubscription = 1 << 3
	SubTransitions EventSubscription = 1 << 4
	SubFilters     EventSubscription = 1 << 5
	SubOutputs     EventSubscription = 1 << 6
	SubSceneItems  EventSubscription = 1 << 7
	SubMediaInputs EventSubscription = 1 << 8
	SubVendors     EventSubscription = 1 << 9
	SubUI          EventSubscription = 1 << 10
	SubAll         EventSubscription = 0x7FF

	// Eventos de alto volume (precisam ser pedidos explicitamente).
	SubInputVolumeMeters         EventSubscription = 1 << 16
	SubInputActiveStateChanged   EventSubscription = 1 << 17
	SubInputShowStateChanged     EventSubscription = 1 << 18
	SubSceneItemTransformChanged EventSubscription = 1 << 19
)

// Códigos de fechamento relevantes (WebSocketCloseCode).
const (
	CloseAuthenticationFailed  = 4009
	CloseUnsupportedRPCVersion = 4010
	CloseSessionInvalidated    = 4011
)

// Códigos de status de requisição relevantes (RequestStatus).
const (
	StatusSuccess              = 100
	StatusMissingRequestField  = 300
	StatusStudioModeNotActive  = 506
	StatusResourceNotFound     = 600
	StatusResourceAlreadyExist = 601
	StatusOutputRunning        = 500
	StatusOutputNotRunning     = 501
)

// Erros de conexão.
var (
	ErrAuthFailed       = errors.New("obs: senha do obs-websocket incorreta")
	ErrAuthRequired     = errors.New("obs: o obs-websocket exige senha e nenhuma foi configurada")
	ErrUnsupportedRPC   = errors.New("obs: versão do protocolo obs-websocket não suportada (requer 5.x)")
	ErrClosed           = errors.New("obs: conexão encerrada")
	ErrHandshakeTimeout = errors.New("obs: tempo esgotado no handshake")
)

// RequestError é retornado quando o OBS responde com result=false.
type RequestError struct {
	RequestType string
	Code        int
	Comment     string
}

func (e *RequestError) Error() string {
	if e.Comment != "" {
		return fmt.Sprintf("obs: %s falhou (código %d): %s", e.RequestType, e.Code, e.Comment)
	}
	return fmt.Sprintf("obs: %s falhou (código %d)", e.RequestType, e.Code)
}

// IsCode informa se err é um RequestError com o código indicado.
func IsCode(err error, code int) bool {
	var re *RequestError
	return errors.As(err, &re) && re.Code == code
}

type envelope struct {
	Op int             `json:"op"`
	D  json.RawMessage `json:"d"`
}

// Hello é a primeira mensagem do servidor.
type Hello struct {
	OBSWebSocketVersion string `json:"obsWebSocketVersion"`
	RPCVersion          int    `json:"rpcVersion"`
	Authentication      *struct {
		Challenge string `json:"challenge"`
		Salt      string `json:"salt"`
	} `json:"authentication,omitempty"`
}

type identify struct {
	RPCVersion         int               `json:"rpcVersion"`
	Authentication     string            `json:"authentication,omitempty"`
	EventSubscriptions EventSubscription `json:"eventSubscriptions"`
}

type identified struct {
	NegotiatedRPCVersion int `json:"negotiatedRpcVersion"`
}

type request struct {
	RequestType string `json:"requestType"`
	RequestID   string `json:"requestId"`
	RequestData any    `json:"requestData,omitempty"`
}

type requestStatus struct {
	Result  bool   `json:"result"`
	Code    int    `json:"code"`
	Comment string `json:"comment,omitempty"`
}

type requestResponse struct {
	RequestType   string          `json:"requestType"`
	RequestID     string          `json:"requestId"`
	RequestStatus requestStatus   `json:"requestStatus"`
	ResponseData  json.RawMessage `json:"responseData,omitempty"`
}

// Event é um evento emitido pelo OBS.
type Event struct {
	Type   string          `json:"eventType"`
	Intent int             `json:"eventIntent"`
	Data   json.RawMessage `json:"eventData,omitempty"`
}

// EventOverflow é um evento sintético entregue quando eventos foram
// descartados por fila cheia; o consumidor deve ressincronizar o estado.
const EventOverflow = "SacraLinkEventOverflow"

// AuthString calcula a string de autenticação do obs-websocket 5.x:
// base64(sha256(base64(sha256(senha + salt)) + challenge)).
func AuthString(password, salt, challenge string) string {
	s := sha256.Sum256([]byte(password + salt))
	secret := base64.StdEncoding.EncodeToString(s[:])
	a := sha256.Sum256([]byte(secret + challenge))
	return base64.StdEncoding.EncodeToString(a[:])
}

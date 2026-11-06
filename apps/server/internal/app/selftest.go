package app

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/gersongabrieldev/sacralink/apps/server/internal/obsbridge"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/platform"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/tlsutil"
)

// Check é o resultado de uma verificação do teste de funcionamento.
type Check struct {
	Name    string `json:"name"`
	Status  string `json:"status"` // ok | warn | error
	Message string `json:"message"`
}

// SelfTest executa o teste de funcionamento do assistente (§4.1, etapa 13):
// verifica de fato cada componente, sem alterar nada.
func (a *App) SelfTest(ctx context.Context) any { return a.selfTest(ctx) }

func (a *App) selfTest(ctx context.Context) []Check {
	var out []Check
	add := func(name, status, msg string) { out = append(out, Check{name, status, msg}) }

	if _, err := a.store.SchemaVersion(ctx); err != nil {
		add("Banco de dados", "error", "não foi possível ler o banco: "+err.Error())
	} else {
		add("Banco de dados", "ok", "banco de dados acessível")
	}

	pctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	err := a.sup.API().Ping(pctx)
	cancel()
	if st := a.sup.Status(); err != nil || st.State != "running" {
		add("Servidor de mídia", "error", fmt.Sprintf("MediaMTX não está respondendo (%s). Use \"Reiniciar servidor de mídia\"; se persistir, veja os logs.", st.State))
	} else {
		add("Servidor de mídia", "ok", "MediaMTX "+st.Version+" em execução")
	}

	host := a.cfg.DeviceHost
	if host == "" {
		host = "127.0.0.1"
	}
	dctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	conn, err := (&tls.Dialer{Config: tlsutil.PinnedClientConfig(a.identity.FingerprintHex)}).DialContext(dctx, "tcp", net.JoinHostPort(host, strconv.Itoa(a.cfg.DevicePort)))
	cancel()
	if err != nil {
		add("API dos celulares", "error", "a porta dos celulares não respondeu com o certificado esperado: "+err.Error())
	} else {
		conn.Close()
		add("API dos celulares", "ok", fmt.Sprintf("TLS 1.3 na porta %d com o certificado deste servidor", a.cfg.DevicePort))
	}

	switch st := a.bridge.Status(); st.State {
	case obsbridge.StateConnected:
		mode := "Studio Mode desligado (trocas diretas pedirão confirmação)"
		if st.StudioMode {
			mode = "Studio Mode ligado"
		}
		add("OBS Studio", "ok", fmt.Sprintf("conectado ao OBS %s · %s", st.OBSVersion, mode))
	case obsbridge.StateAuthFailed:
		add("OBS Studio", "error", "senha do obs-websocket incorreta")
	case obsbridge.StateDisabled:
		add("OBS Studio", "warn", "OBS ainda não configurado")
	default:
		add("OBS Studio", "warn", "OBS não conectado — abra o OBS e habilite o servidor WebSocket")
	}

	missing := 0
	for _, r := range a.firewallRules() {
		if !platform.FirewallRuleExists(r.Name) {
			missing++
		}
	}
	if missing > 0 {
		add("Firewall do Windows", "warn", fmt.Sprintf("%d regra(s) ausente(s): os celulares podem não conseguir conectar", missing))
	} else {
		add("Firewall do Windows", "ok", "regras do SacraLink presentes")
	}

	if hosts := a.Endpoints().LANHosts; len(hosts) == 0 {
		add("Rede local", "error", "nenhum endereço de rede local encontrado")
	} else {
		add("Rede local", "ok", "celulares devem conectar em "+hosts[0])
	}

	osInfo := platform.OS()
	switch {
	case osInfo.Warning != "":
		add("Windows", "warn", osInfo.Warning)
	case osInfo.SmartAppCtrl == "on":
		add("Windows", "ok", osInfo.Name+" · Smart App Control ativo: use apenas instaladores assinados do SacraLink")
	default:
		add("Windows", "ok", osInfo.Name)
	}
	return out
}

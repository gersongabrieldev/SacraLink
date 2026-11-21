// Package update avisa o operador quando há uma versão nova do SacraLink.
//
// Consulta a lista pública de releases do GitHub ao iniciar e a cada 12 h.
// Não envia nada além do User-Agent ("SacraLink/<versão>"), não baixa e não
// instala nada sozinho: o painel só mostra o aviso com o link do instalador.
// Pode ser desligado nas configurações ("Verificar atualizações"); sem
// internet, a falha é silenciosa (fica registrada em Status.Error).
package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DefaultURL é a lista de releases do repositório oficial.
const DefaultURL = "https://api.github.com/repos/gersongabrieldev/SacraLink/releases?per_page=20"

// InstallerAsset é o nome do instalador anexado a cada release.
const InstallerAsset = "SacraLink-Setup-x64.exe"

const (
	interval    = 12 * time.Hour
	firstDelay  = 15 * time.Second
	minManual   = time.Minute // intervalo mínimo entre verificações pedidas no painel
	maxBodySize = 2 << 20
)

// Release é uma versão publicada.
type Release struct {
	Version      string `json:"version"`
	Name         string `json:"name"`
	URL          string `json:"url"`
	InstallerURL string `json:"installerUrl,omitempty"`
	PublishedAt  int64  `json:"publishedAt"`
	Prerelease   bool   `json:"prerelease"`
}

// Status é o resultado da última verificação.
type Status struct {
	Current   string   `json:"current"`
	Enabled   bool     `json:"enabled"`
	Available bool     `json:"available"`
	Latest    *Release `json:"latest,omitempty"`
	CheckedAt int64    `json:"checkedAt,omitempty"`
	Error     string   `json:"error,omitempty"`
}

// ErrDevBuild: build de desenvolvimento/CI (versão 0.0.0-...), sem verificação.
var ErrDevBuild = errors.New("versão de desenvolvimento: verificação de atualizações desativada")

// Checker consulta as releases. Campos exportados configuram antes de Run.
type Checker struct {
	Current string
	URL     string
	Client  *http.Client
	Enabled func() bool
	Log     *slog.Logger

	mu     sync.Mutex
	status Status
	last   time.Time
}

// New cria o verificador para a versão em execução.
func New(current string, enabled func() bool, log *slog.Logger) *Checker {
	if log == nil {
		log = slog.Default()
	}
	return &Checker{Current: current, URL: DefaultURL, Client: &http.Client{Timeout: 15 * time.Second},
		Enabled: enabled, Log: log.With("componente", "atualizacoes")}
}

// Status retorna o resultado da última verificação.
func (c *Checker) Status() Status {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.status
	s.Current, s.Enabled = c.Current, c.enabled()
	if !s.Enabled {
		s.Available = false
	}
	return s
}

func (c *Checker) enabled() bool { return c.Enabled == nil || c.Enabled() }

// Run verifica logo após iniciar e depois a cada 12 h, enquanto habilitado.
func (c *Checker) Run(ctx context.Context) {
	if _, ok := parse(c.Current); !ok || strings.HasPrefix(c.Current, "0.0.0") {
		return
	}
	wait := firstDelay
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		wait = interval
		if !c.enabled() {
			continue
		}
		if _, err := c.check(ctx); err != nil && ctx.Err() == nil {
			c.Log.Info("verificação de atualizações falhou", "erro", err)
		}
	}
}

// CheckNow verifica agora (pedido do painel), no máximo uma vez por minuto.
// Desligado nas configurações, não consulta nada.
func (c *Checker) CheckNow(ctx context.Context) (Status, error) {
	c.mu.Lock()
	recent := time.Since(c.last) < minManual
	c.mu.Unlock()
	if recent || !c.enabled() {
		return c.Status(), nil
	}
	return c.check(ctx)
}

func (c *Checker) check(ctx context.Context) (Status, error) {
	cur, ok := parse(c.Current)
	if !ok || strings.HasPrefix(c.Current, "0.0.0") {
		return c.Status(), ErrDevBuild
	}
	c.mu.Lock()
	c.last = time.Now()
	c.mu.Unlock()
	latest, err := c.fetch(ctx, cur)
	c.mu.Lock()
	c.status.CheckedAt = time.Now().UnixMilli()
	if err != nil {
		c.status.Error = err.Error()
	} else {
		c.status.Error = ""
		c.status.Latest = latest
		c.status.Available = latest != nil
	}
	c.mu.Unlock()
	if err == nil && latest != nil {
		c.Log.Info("nova versão disponível", "atual", c.Current, "nova", latest.Version)
	}
	return c.Status(), err
}

type ghRelease struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	HTMLURL     string    `json:"html_url"`
	Draft       bool      `json:"draft"`
	Prerelease  bool      `json:"prerelease"`
	PublishedAt time.Time `json:"published_at"`
	Assets      []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

// fetch devolve a maior versão publicada acima da atual (nil se não houver).
func (c *Checker) fetch(ctx context.Context, cur version) (*Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.URL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "SacraLink/"+c.Current)
	res, err := c.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sem acesso ao GitHub: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub respondeu %s", res.Status)
	}
	var list []ghRelease
	if err := json.NewDecoder(io.LimitReader(res.Body, maxBodySize)).Decode(&list); err != nil {
		return nil, fmt.Errorf("resposta inválida do GitHub: %w", err)
	}
	// Quem está numa versão 0.x ou de pré-lançamento também recebe
	// pré-lançamentos; numa versão estável, só versões estáveis.
	withPre := cur.major == 0 || cur.pre != ""
	var best *Release
	var bestV version
	for _, r := range list {
		v, ok := parse(r.TagName)
		if r.Draft || !ok || (!withPre && (r.Prerelease || v.pre != "")) || compare(v, cur) <= 0 {
			continue
		}
		if best != nil && compare(v, bestV) <= 0 {
			continue
		}
		rel := &Release{Version: strings.TrimPrefix(r.TagName, "v"), Name: r.Name, URL: githubURL(r.HTMLURL),
			PublishedAt: r.PublishedAt.UnixMilli(), Prerelease: r.Prerelease || v.pre != ""}
		for _, a := range r.Assets {
			if a.Name == InstallerAsset {
				rel.InstallerURL = githubURL(a.URL)
			}
		}
		best, bestV = rel, v
	}
	return best, nil
}

// githubURL só repassa ao painel endereços https do GitHub ("" nos demais).
func githubURL(u string) string {
	if strings.HasPrefix(u, "https://github.com/") {
		return u
	}
	return ""
}

// version é uma versão semântica (vX.Y.Z[-pré][+build]).
type version struct {
	major, minor, patch int
	pre                 string
}

func parse(s string) (version, bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	if i := strings.IndexByte(s, '+'); i >= 0 {
		s = s[:i]
	}
	var v version
	core := s
	if i := strings.IndexByte(s, '-'); i >= 0 {
		core, v.pre = s[:i], s[i+1:]
		if v.pre == "" {
			return v, false
		}
	}
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return v, false
	}
	nums := [3]*int{&v.major, &v.minor, &v.patch}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return v, false
		}
		*nums[i] = n
	}
	return v, true
}

// compare segue a precedência do SemVer 2.0 (1.0.0-rc.1 < 1.0.0).
func compare(a, b version) int {
	for _, d := range [][2]int{{a.major, b.major}, {a.minor, b.minor}, {a.patch, b.patch}} {
		if d[0] != d[1] {
			if d[0] < d[1] {
				return -1
			}
			return 1
		}
	}
	switch {
	case a.pre == b.pre:
		return 0
	case a.pre == "":
		return 1
	case b.pre == "":
		return -1
	}
	ap, bp := strings.Split(a.pre, "."), strings.Split(b.pre, ".")
	for i := 0; i < len(ap) && i < len(bp); i++ {
		if ap[i] == bp[i] {
			continue
		}
		an, aerr := strconv.Atoi(ap[i])
		bn, berr := strconv.Atoi(bp[i])
		switch {
		case aerr == nil && berr == nil:
			if an < bn {
				return -1
			}
			return 1
		case aerr == nil:
			return -1 // numérico < alfanumérico
		case berr == nil:
			return 1
		case ap[i] < bp[i]:
			return -1
		default:
			return 1
		}
	}
	switch {
	case len(ap) < len(bp):
		return -1
	case len(ap) > len(bp):
		return 1
	}
	return 0
}

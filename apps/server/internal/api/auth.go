package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gersongabrieldev/sacralink/apps/server/internal/ratelimit"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/secure"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/store"
)

// Parâmetros de sessão do administrador.
const (
	SessionTTL    = 12 * time.Hour
	TicketTTL     = 30 * time.Second
	MaxLoginFails = 5
	LoginLockout  = 5 * time.Minute
	AdminUsername = "admin"
)

var errLoginFailed = &Error{http.StatusUnauthorized, "login_failed", "usuário ou senha incorretos"}

// Auth gerencia sessões do administrador (em memória: reiniciar o servidor
// exige novo login).
type Auth struct {
	st      *store.Store
	limiter *ratelimit.Limiter

	mu       sync.Mutex
	sessions map[string]*adminSession // hash do token → sessão
	tickets  map[string]ticket
}

type adminSession struct {
	user    string
	expires time.Time
}

type ticket struct {
	user    string
	expires time.Time
}

// NewAuth cria o gerenciador de autenticação.
func NewAuth(st *store.Store) *Auth {
	return &Auth{
		st:       st,
		limiter:  ratelimit.New(5, 10, 10, 5*time.Minute, 5*time.Minute),
		sessions: map[string]*adminSession{},
		tickets:  map[string]ticket{},
	}
}

// Configured informa se já existe senha de administrador.
func (a *Auth) Configured(ctx context.Context) (bool, error) {
	n, err := a.st.CountUsers(ctx)
	return n > 0, err
}

// Setup cria o administrador (apenas se ainda não existir).
func (a *Auth) Setup(ctx context.Context, password, ip string) (string, error) {
	ok, err := a.Configured(ctx)
	if err != nil {
		return "", err
	}
	if ok {
		return "", &Error{http.StatusConflict, "already_configured", "o administrador já foi configurado"}
	}
	hash, err := secure.HashPassword(password)
	if err != nil {
		return "", badRequest(err.Error())
	}
	if _, err := a.st.CreateUser(ctx, AdminUsername, hash, "admin"); err != nil {
		return "", err
	}
	_ = a.st.Audit(ctx, AdminUsername, "auth.setup", "", ip, true, "")
	return a.newSession(AdminUsername), nil
}

// Login valida a senha com limite por IP e bloqueio da conta.
func (a *Auth) Login(ctx context.Context, username, password, ip string) (string, error) {
	if !a.limiter.Allow(ip) {
		_ = a.st.Audit(ctx, username, "auth.login", "", ip, false, "ip bloqueado")
		return "", &Error{http.StatusTooManyRequests, "rate_limited", "muitas tentativas; aguarde alguns minutos"}
	}
	if username == "" {
		username = AdminUsername
	}
	u, err := a.st.GetUserByName(ctx, username)
	if errors.Is(err, store.ErrNotFound) {
		secure.CheckPassword(dummyHash(), password) // mesmo custo de uma senha real
		a.limiter.Fail(ip)
		return "", errLoginFailed
	}
	if err != nil {
		return "", err
	}
	if time.Now().UnixMilli() < u.LockedUntil {
		_ = a.st.Audit(ctx, username, "auth.login", "", ip, false, "conta bloqueada")
		return "", &Error{http.StatusTooManyRequests, "account_locked", "conta bloqueada temporariamente por tentativas incorretas"}
	}
	if !secure.CheckPassword(u.PasswordHash, password) {
		a.limiter.Fail(ip)
		_ = a.st.RecordLoginFailure(ctx, u.ID, MaxLoginFails, LoginLockout.Milliseconds())
		_ = a.st.Audit(ctx, username, "auth.login", "", ip, false, "senha incorreta")
		return "", errLoginFailed
	}
	a.limiter.Success(ip)
	_ = a.st.RecordLoginSuccess(ctx, u.ID)
	_ = a.st.Audit(ctx, username, "auth.login", "", ip, true, "")
	return a.newSession(username), nil
}

// ChangePassword troca a senha (exige a senha atual) e encerra outras sessões.
func (a *Auth) ChangePassword(ctx context.Context, user, current, next, ip string) (string, error) {
	u, err := a.st.GetUserByName(ctx, user)
	if err != nil {
		return "", err
	}
	if !secure.CheckPassword(u.PasswordHash, current) {
		a.limiter.Fail(ip)
		return "", errLoginFailed
	}
	hash, err := secure.HashPassword(next)
	if err != nil {
		return "", badRequest(err.Error())
	}
	if err := a.st.UpdatePassword(ctx, u.ID, hash); err != nil {
		return "", err
	}
	a.mu.Lock()
	a.sessions = map[string]*adminSession{}
	a.mu.Unlock()
	_ = a.st.Audit(ctx, user, "auth.password_change", "", ip, true, "")
	return a.newSession(user), nil
}

func (a *Auth) newSession(user string) string {
	tok := secure.NewToken()
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now()
	for k, s := range a.sessions {
		if now.After(s.expires) {
			delete(a.sessions, k)
		}
	}
	a.sessions[secure.HashSecret(tok)] = &adminSession{user: user, expires: now.Add(SessionTTL)}
	return tok
}

// Validate valida o token (validade deslizante).
func (a *Auth) Validate(token string) (string, bool) {
	if token == "" {
		return "", false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	s := a.sessions[secure.HashSecret(token)]
	if s == nil || time.Now().After(s.expires) {
		return "", false
	}
	s.expires = time.Now().Add(SessionTTL)
	return s.user, true
}

// Logout encerra a sessão.
func (a *Auth) Logout(token string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.sessions, secure.HashSecret(token))
}

// NewTicket cria um ticket de uso único para abrir o WebSocket do painel.
func (a *Auth) NewTicket(user string) string {
	t := secure.NewToken()
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now()
	for k, v := range a.tickets {
		if now.After(v.expires) {
			delete(a.tickets, k)
		}
	}
	a.tickets[t] = ticket{user: user, expires: now.Add(TicketTTL)}
	return t
}

// UseTicket consome o ticket.
func (a *Auth) UseTicket(t string) (string, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	v, ok := a.tickets[t]
	delete(a.tickets, t)
	if !ok || time.Now().After(v.expires) {
		return "", false
	}
	return v.user, true
}

func bearer(r *http.Request) string {
	v, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	return strings.TrimSpace(v)
}

type ctxKey struct{}

func userFrom(ctx context.Context) string {
	u, _ := ctx.Value(ctxKey{}).(string)
	return u
}

// require exige sessão de administrador válida.
func (a *Auth) require(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := a.Validate(bearer(r))
		if !ok {
			writeError(w, &Error{http.StatusUnauthorized, "unauthorized", "faça login novamente"})
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, user)))
	}
}

var (
	dummyOnce sync.Once
	dummy     string
)

// dummyHash é um hash bcrypt real usado para igualar o tempo de resposta
// quando o usuário não existe.
func dummyHash() string {
	dummyOnce.Do(func() { dummy, _ = secure.HashPassword("senha-ficticia-para-tempo-constante") })
	return dummy
}

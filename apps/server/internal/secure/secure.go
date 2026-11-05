// Package secure reúne primitivas de segurança usadas pelo servidor:
// geração de identificadores e segredos, hash de segredos de dispositivos,
// hash de senhas de administrador e comparação em tempo constante.
package secure

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// alfabeto base32 minúsculo sem padding, seguro para paths do MediaMTX e URLs.
var idEncoding = base32.NewEncoding("abcdefghijklmnopqrstuvwxyz234567").WithPadding(base32.NoPadding)

// RandomBytes retorna n bytes criptograficamente aleatórios.
func RandomBytes(n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand não falha em sistemas suportados; falhar aqui é mais
		// seguro do que continuar com entropia insuficiente.
		panic("secure: falha ao ler crypto/rand: " + err.Error())
	}
	return b
}

// NewID gera um identificador com prefixo e n caracteres base32 minúsculos.
// Ex.: NewID("c", 11) → "c4k2m7q..." (sem caracteres ambíguos de URL).
func NewID(prefix string, n int) string {
	raw := RandomBytes((n*5 + 7) / 8)
	s := idEncoding.EncodeToString(raw)
	return prefix + s[:n]
}

// NewToken gera um segredo aleatório de 32 bytes codificado em base64url.
func NewToken() string {
	return base64.RawURLEncoding.EncodeToString(RandomBytes(32))
}

// HashSecret retorna o SHA-256 (hex) de um segredo de alta entropia.
// Adequado para tokens aleatórios; NÃO use para senhas escolhidas por humanos.
func HashSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

// EqualHash compara dois hashes hex em tempo constante.
func EqualHash(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// Fingerprint retorna o SHA-256 em hex maiúsculo separado por ':' (formato
// usual de impressões digitais de certificados).
func Fingerprint(der []byte) string {
	sum := sha256.Sum256(der)
	h := strings.ToUpper(hex.EncodeToString(sum[:]))
	var b strings.Builder
	for i := 0; i < len(h); i += 2 {
		if i > 0 {
			b.WriteByte(':')
		}
		b.WriteString(h[i : i+2])
	}
	return b.String()
}

// Erros de política de senha.
var (
	ErrPasswordTooShort = errors.New("a senha deve ter pelo menos 8 caracteres")
	ErrPasswordTooLong  = errors.New("a senha deve ter no máximo 72 bytes")
)

// ValidatePassword aplica a política mínima de senha do administrador.
func ValidatePassword(pw string) error {
	if len([]rune(pw)) < 8 {
		return ErrPasswordTooShort
	}
	if len(pw) > 72 { // limite do bcrypt
		return ErrPasswordTooLong
	}
	return nil
}

// HashPassword gera um hash bcrypt (custo 12) da senha.
func HashPassword(pw string) (string, error) {
	if err := ValidatePassword(pw); err != nil {
		return "", err
	}
	h, err := bcrypt.GenerateFromPassword([]byte(pw), 12)
	if err != nil {
		return "", err
	}
	return string(h), nil
}

// CheckPassword verifica a senha contra o hash bcrypt.
func CheckPassword(hash, pw string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
}

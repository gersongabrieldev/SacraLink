// Package tlsutil gera e carrega o certificado TLS autoassinado da API dos
// dispositivos. A confiança não depende de CA: o aplicativo móvel fixa
// (pinning) a impressão digital SHA-256 recebida no QR Code de pareamento.
package tlsutil

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

// Identity é o certificado do servidor e sua impressão digital.
type Identity struct {
	Certificate tls.Certificate
	// FingerprintHex é o SHA-256 do certificado (DER) em hex minúsculo.
	FingerprintHex string
	NotAfter       time.Time
}

// LoadOrCreate carrega dir/server.crt e dir/server.key ou gera um novo par.
// O certificado NÃO é regenerado quando os IPs mudam, porque isso quebraria o
// pinning dos aparelhos já pareados; os nomes servem apenas para diagnóstico.
func LoadOrCreate(dir, commonName string) (*Identity, error) {
	certPath, keyPath := filepath.Join(dir, "server.crt"), filepath.Join(dir, "server.key")
	if id, err := load(certPath, keyPath); err == nil {
		return id, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("certificado TLS inválido (apague %s para gerar outro e pareie os aparelhos novamente): %w", dir, err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	certPEM, keyPEM, err := Generate(commonName, 10*365*24*time.Hour)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		return nil, err
	}
	if err := os.WriteFile(certPath, certPEM, 0o644); err != nil {
		return nil, err
	}
	return load(certPath, keyPath)
}

func load(certPath, keyPath string) (*Identity, error) {
	if _, err := os.Stat(certPath); err != nil {
		return nil, err
	}
	c, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, err
	}
	leaf, err := x509.ParseCertificate(c.Certificate[0])
	if err != nil {
		return nil, err
	}
	c.Leaf = leaf
	return &Identity{Certificate: c, FingerprintHex: Fingerprint(c.Certificate[0]), NotAfter: leaf.NotAfter}, nil
}

// Fingerprint retorna o SHA-256 do DER em hex minúsculo.
func Fingerprint(der []byte) string {
	s := sha256.Sum256(der)
	return hex.EncodeToString(s[:])
}

// Generate cria um par certificado/chave ECDSA P-256 autoassinado.
func Generate(commonName string, validity time.Duration) (certPEM, keyPEM []byte, err error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
	if err != nil {
		return nil, nil, err
	}
	host, _ := os.Hostname()
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: commonName, Organization: []string{"SacraLink"}},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(validity),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost", "sacralink.local"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
	}
	if host != "" {
		tmpl.DNSNames = append(tmpl.DNSNames, host)
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}
	kb, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: kb}), nil
}

// ServerConfig retorna a configuração TLS 1.3 do servidor de dispositivos.
func (id *Identity) ServerConfig() *tls.Config {
	return &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{id.Certificate},
	}
}

// PinnedClientConfig retorna uma configuração de cliente que aceita apenas o
// certificado com a impressão digital informada (usado em testes e pelo
// publicador de teste; o app móvel implementa o mesmo em Dart).
func PinnedClientConfig(fingerprintHex string) *tls.Config {
	return &tls.Config{
		MinVersion:         tls.VersionTLS13,
		InsecureSkipVerify: true, // a verificação é feita pelo pinning abaixo
		VerifyPeerCertificate: func(raw [][]byte, _ [][]*x509.Certificate) error {
			if len(raw) == 0 {
				return errors.New("servidor não apresentou certificado")
			}
			if Fingerprint(raw[0]) != fingerprintHex {
				return errors.New("impressão digital do certificado não confere (possível ataque ou servidor diferente)")
			}
			return nil
		},
	}
}

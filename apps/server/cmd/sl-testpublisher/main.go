// Comando sl-testpublisher: simula N celulares publicando vídeo H.264 e
// áudio Opus sintéticos no SacraLink, para testes ponta a ponta e de carga
// (especificação §19.2). Não substitui testes com aparelhos reais.
//
// Exemplos:
//
//	# pareia 4 câmeras usando a senha do administrador e publica por 5 minutos
//	sl-testpublisher --admin-password "..." --count 4 --duration 5m
//
//	# reutiliza câmeras já pareadas (arquivo gerado na primeira execução)
//	sl-testpublisher --creds testpub-creds.json --duration 30m
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/gersongabrieldev/sacralink/apps/server/internal/testpub"
)

func main() {
	adminURL := flag.String("admin-url", "http://127.0.0.1:7380", "API de administração")
	adminPass := flag.String("admin-password", "", "senha do administrador (para gerar pareamentos)")
	count := flag.Int("count", 1, "número de câmeras simuladas")
	credsFile := flag.String("creds", "testpub-creds.json", "arquivo de credenciais (reutilizado se existir)")
	host := flag.String("host", "127.0.0.1", "endereço do servidor para os aparelhos")
	duration := flag.Duration("duration", time.Minute, "duração da publicação")
	width := flag.Int("width", 320, "largura (múltiplo de 16)")
	height := flag.Int("height", 240, "altura (múltiplo de 16)")
	fps := flag.Int("fps", 15, "quadros por segundo")
	loopback := flag.Bool("loopback", true, "permitir candidatos ICE em 127.0.0.1")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	creds, err := loadOrPair(ctx, *credsFile, *adminURL, *adminPass, *host, *count)
	if err != nil {
		fmt.Fprintln(os.Stderr, "erro:", err)
		os.Exit(1)
	}
	fmt.Printf("Publicando %d câmera(s) %dx%d@%d por %s (Ctrl+C para parar)\n", len(creds), *width, *height, *fps, *duration)

	var wg sync.WaitGroup
	for i, c := range creds {
		wg.Add(1)
		go func(i int, c *testpub.Credentials) {
			defer wg.Done()
			ctl, err := testpub.Connect(ctx, c)
			if err != nil {
				fmt.Printf("[cam %d] controle: %v\n", i+1, err)
				return
			}
			defer ctl.Close()
			pub, err := testpub.Publish(ctx, c, testpub.PublishOptions{Width: *width, Height: *height, FPS: *fps, IncludeLoopback: *loopback})
			if err != nil {
				fmt.Printf("[cam %d] WHIP: %v\n", i+1, err)
				return
			}
			defer pub.Close()
			fmt.Printf("[cam %d] %s publicando\n", i+1, c.CameraName)
			t := time.NewTicker(10 * time.Second)
			defer t.Stop()
			end := time.After(*duration)
			for {
				select {
				case <-ctx.Done():
					return
				case <-end:
					return
				case <-t.C:
					tl, _ := ctl.LastTally()
					fmt.Printf("[cam %d] conectado=%v quadros=%d tally=%s\n", i+1, pub.Connected(), pub.Frames, tl.State)
				}
			}
		}(i, c)
	}
	wg.Wait()
}

func loadOrPair(ctx context.Context, file, adminURL, adminPass, host string, count int) ([]*testpub.Credentials, error) {
	var creds []*testpub.Credentials
	if b, err := os.ReadFile(file); err == nil {
		if err := json.Unmarshal(b, &creds); err != nil {
			return nil, fmt.Errorf("arquivo de credenciais inválido: %w", err)
		}
	}
	if len(creds) >= count {
		return creds[:count], nil
	}
	if adminPass == "" {
		return nil, fmt.Errorf("são necessárias %d câmeras e há %d pareadas: informe --admin-password", count, len(creds))
	}
	token, err := login(ctx, adminURL, adminPass)
	if err != nil {
		return nil, err
	}
	for len(creds) < count {
		uri, err := createPairing(ctx, adminURL, token)
		if err != nil {
			return nil, err
		}
		c, err := testpub.Pair(ctx, uri, fmt.Sprintf("Simulador %02d", len(creds)+1), host)
		if err != nil {
			return nil, err
		}
		creds = append(creds, c)
	}
	b, _ := json.MarshalIndent(creds, "", "  ")
	if err := os.WriteFile(file, b, 0o600); err != nil {
		return nil, err
	}
	return creds, nil
}

func postJSON(ctx context.Context, url, token string, in, out any) error {
	b, _ := json.Marshal(in)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("%s: HTTP %d", url, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func login(ctx context.Context, base, pass string) (string, error) {
	var r struct{ Token string }
	err := postJSON(ctx, base+"/api/v1/auth/login", "", map[string]string{"password": pass}, &r)
	return r.Token, err
}

func createPairing(ctx context.Context, base, token string) (string, error) {
	var r struct{ URI string }
	err := postJSON(ctx, base+"/api/v1/devices/pair", token, map[string]string{"label": "simulador"}, &r)
	return r.URI, err
}

# Ambiente de desenvolvimento

## Ferramentas (somente desenvolvimento)

| Ferramenta | Versão testada | Para quê |
|---|---|---|
| Go | 1.27 | Servidor e painel |
| Node.js + npm | 24 / 11 | Build do painel (React/Vite) |
| Flutter | 3.47 (estável) + JDK 17 + Android SDK | App móvel |
| Xcode (macOS) | — | iOS |
| Inno Setup | 6 | Instalador (o CI de release instala) |
| OBS Studio | 28+ | Testes manuais |

Nada disso é necessário no computador do usuário final. A CLI do Wails é
**opcional**: o painel compila com `go build -tags desktop,production`.

## Estrutura

```
apps/server     Go: sacralink-server (APIs, OBS, MediaMTX, núcleo, bandeja)
  cmd/sacralink-server   executável principal
  cmd/sl-testpublisher   celulares sintéticos (teste ponta a ponta e carga)
  cmd/sl-fakeobs         OBS simulado para desenvolver o painel
  cmd/sl-genicons        gera os ícones a partir da identidade em código
  internal/…             pacotes (ver docs/architecture/00-plano-tecnico.md)
apps/desktop    Wails v2: SacraLink.exe; frontend/ (React + TS + Tailwind)
apps/mobile     Flutter: SacraLink Camera; packages/sacralink_device_caps (nativo)
packages/contracts  contrato do protocolo de controle
installer/windows   Inno Setup
scripts/        build, MediaMTX, bootstrap do app, utilitários de dev
```

## Primeiros passos

```bash
pwsh scripts/fetch-mediamtx.ps1          # MediaMTX v1.21.3 com SHA-256 conferido
scripts/dev/test-server.sh               # testes do servidor (inclui ponta a ponta)
cd apps/desktop/frontend && npm ci && npm test && npm run build
```

### Rodar o servidor em modo desenvolvimento

```bash
cd apps/server
go run ./cmd/sacralink-server --dev --no-tray \
   --data-dir ../../.devdata --device-host 127.0.0.1 --ice-host 127.0.0.1
```

`--device-host/--ice-host 127.0.0.1` evitam o alerta do Firewall do Windows
durante o desenvolvimento. `--dev` aceita o painel do Vite
(`npm run dev` em `apps/desktop/frontend`, http://localhost:5173).

### Demonstração completa sem celular nem OBS

```bash
cd apps/server
SACRALINK_DEMO=1 SACRALINK_DEMO_CAMERAS=4 \
SACRALINK_DEMO_DASHBOARD=../desktop/frontend/dist \
go test -run TestDemo -timeout 30m ./internal/app/
# abra http://127.0.0.1:17380 (senha demo-admin-123)
```

Capturas de tela: `node scripts/dev/screenshot.mjs <url> <token> saida.png`.

### Build Windows completo

```powershell
pwsh scripts/build-windows.ps1 -Version 0.1.0
iscc /DAppVersion=0.1.0 installer\windows\SacraLink.iss   # se o Inno Setup estiver instalado
```

### App móvel

Ver [docs/mobile/README.md](mobile/README.md).

## Smart App Control (Windows 11)

Em máquinas com o **Smart App Control** ligado, o Windows bloqueia executáveis
sem assinatura e sem reputação, inclusive os binários de teste que o Go gera
(`An Application Control policy has blocked this file`). Não desligue o Smart
App Control:

- aponte o temporário do Go para o perfil (`GOTMPDIR=$USERPROFILE/.gotmp`);
- use `scripts/dev/test-server.sh`, que repete **apenas** os bloqueios
  intermitentes (falhas reais de teste falham na hora).

**Impacto no produto:** ver [assinatura de código](installation/assinatura.md).

## Convenções

- Português do Brasil em documentação, comentários, interface e commits
  (Conventional Commits).
- `gofmt`, `go vet`, testes verdes e `npm run build` antes de cada commit.
- O CI roda o servidor no Linux com `-race` e no Windows, o painel, o build
  Windows, o app Android (análise/testes/APK) e o build iOS sem assinatura.

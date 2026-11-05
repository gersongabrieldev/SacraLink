# Ambiente de desenvolvimento

## Ferramentas (somente desenvolvimento)

| Ferramenta | Versão | Para quê |
|---|---|---|
| Go | 1.27+ | Servidor e painel |
| Node.js + npm | 20+ (testado com 24) | Build do frontend |
| Wails CLI | v2.17 (`go install github.com/wailsapp/wails/v2/cmd/wails@v2.17.1`) | Build do painel |
| Flutter | 3.x estável | App móvel |
| Android Studio / SDK + JDK 17 | | Build Android |
| Xcode (macOS) | | Build iOS |
| Inno Setup 6 | | Instalador |
| OBS Studio | 28+ | Testes de integração manuais |

Nada disso é necessário no computador do usuário final.

## Estrutura

Veja a seção 10 do [plano técnico](architecture/00-plano-tecnico.md).

## Smart App Control (Windows 11)

Em máquinas com o **Smart App Control** ligado, o Windows bloqueia executáveis
sem assinatura e sem reputação, inclusive os binários de teste que o Go gera em
`%TEMP%` (erro `An Application Control policy has blocked this file`).
Não desligue o Smart App Control: aponte o diretório temporário do Go para o seu
perfil, o que resolveu o problema no ambiente de referência:

```bash
export GOTMPDIR="$USERPROFILE/.gotmp"   # Git Bash
$env:GOTMPDIR = "$env:USERPROFILE\.gotmp" # PowerShell
```

**Impacto no produto:** em PCs de igrejas com Smart App Control ligado, o
instalador e os executáveis (SacraLink e MediaMTX) só rodam se estiverem
**assinados com certificado de code signing confiável** (ou com reputação
estabelecida). Por isso a assinatura de código é requisito para a distribuição
pública (ver `docs/installation/assinatura.md`).

## Servidor (Go)

```bash
cd apps/server
go test ./...
```

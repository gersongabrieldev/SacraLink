# Guia de instalação (computador da igreja)

## Requisitos

- Windows 11 (recomendado) ou Windows 10 1809+, **64 bits**.
  O Windows 10 encerrou o suporte em 14/10/2025: funciona, mas exige cuidado
  extra se o computador for exposto à internet.
- OBS Studio 28 ou superior (já inclui o obs-websocket 5).
- Roteador Wi-Fi 5 GHz; computador **no cabo** de rede, de preferência.
- WebView2 Runtime (já vem no Windows 11; o instalador instala se faltar).

Não é necessário instalar Go, Node.js, Python, Docker ou WSL.

## Instalação

1. Baixe `SacraLink-Setup-x64.exe` em
   [Releases](https://github.com/gersongabrieldev/SacraLink/releases) e confira
   o SHA-256 em `SHA256SUMS.txt`.
2. Execute o instalador (pede permissão de administrador).
3. Mantenha marcado **Configurar o Firewall do Windows** — cria apenas:
   TCP 7443 (aparelhos) e UDP/TCP 8189 (mídia WebRTC), para redes privadas
   e de domínio. O painel e o OBS continuam acessíveis só no próprio PC.
4. Ao final, abra o SacraLink. O assistente pede:
   - nome da igreja e **senha do administrador**;
   - conexão com o OBS (botão *Importar configuração do OBS deste PC*);
   - autorização para criar fontes/cenas "SacraLink — …" no OBS;
   - regras de firewall (se não marcadas no instalador).

## Funcionamento

- `sacralink-server.exe` fica na **bandeja** (perto do relógio) e continua
  recebendo câmeras mesmo com o painel fechado.
- `SacraLink.exe` é o painel. Fechá-lo não derruba as câmeras.
- *Sair do SacraLink* na bandeja encerra tudo (pede confirmação se houver
  câmeras transmitindo).
- Iniciar com o Windows: *Configurações → Sistema*.

## Dados, logs e recuperação

| O quê | Onde |
|---|---|
| Banco, certificados, configuração do MediaMTX | `%LOCALAPPDATA%\SacraLink` |
| Logs (rotação 10 MB × 10, 30 dias) | `%LOCALAPPDATA%\SacraLink\logs` |
| Programa | `C:\Program Files\SacraLink` |

- **Esqueci a senha:** feche o SacraLink e execute
  `"C:\Program Files\SacraLink\sacralink-server.exe" admin reset --confirm`.
  Na próxima abertura o assistente cria uma nova senha (aparelhos e câmeras
  são mantidos).
- **Atualizar:** rode o instalador da nova versão; os dados são preservados.
- **Desinstalar:** Configurações do Windows → Aplicativos. As regras de
  firewall são removidas; o desinstalador pergunta se deve apagar os dados.

## Problemas comuns

| Sintoma | Causa provável |
|---|---|
| "Porta em uso" ao iniciar | Outro SacraLink aberto, ou outro programa nas portas 7380/7443/8189/8554 |
| Celular não encontra o servidor | Celular em outra rede (ex.: Wi-Fi de visitantes isolado) ou firewall sem as regras |
| "Senha do OBS incorreta" | Senha do obs-websocket mudou: use *Importar configuração do OBS deste PC* |
| Windows bloqueou o programa | Smart App Control/SmartScreen com executável não assinado (ver [assinatura.md](assinatura.md)) |

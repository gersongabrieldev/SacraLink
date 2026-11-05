# Acesso externo (4G/5G) sem VPS

O SacraLink **não usa** VPS, relay ou servidor em nuvem. Para um celular em
4G/5G (procissões, caminhadas) transmitir, ele precisa **alcançar
diretamente** o computador da igreja. Isso depende da internet da igreja.

## Cenários

| Cenário | Funciona? | O que fazer |
|---|---|---|
| IPv4 público (fixo ou dinâmico) | ✅ | Encaminhar portas no roteador e, se o IP mudar, usar DNS dinâmico |
| IPv6 global no PC e no celular | ✅ | Liberar entrada das portas no firewall do roteador para o IPv6 do PC |
| CGNAT (IP WAN 100.64.x.x ou privado) **sem** IPv6 | ❌ | Inviável sem relay. Pedir IP público ou IPv6 à operadora / plano empresarial |
| Dupla NAT (dois roteadores) | ⚠️ | Encaminhar portas nos dois, ou colocar o modem em modo bridge |

> STUN apenas **descobre** o IP público; ele não cria rota de entrada.
> Um servidor TURN local também precisaria estar acessível de fora — não
> resolve a ausência de rota pública.

## Passo a passo (IPv4 público)

1. **Rede → Diagnosticar** no painel: anote o *IP local* (ex.: 192.168.0.10)
   e o *IP público*.
2. Compare o IP público com o "IP WAN" mostrado no roteador. Se forem
   diferentes, ou se o do roteador começar com 100.64–100.127, você está
   atrás de **CGNAT**.
3. No roteador, encaminhe para o IP local do PC:
   - **TCP 7443** (aparelhos/sinalização)
   - **UDP 8189** e **TCP 8189** (mídia WebRTC)
   Nunca encaminhe 7380 (painel), 4455 (OBS), 8554, 8889 ou 9997.
4. (Opcional) Configure um DNS dinâmico (DuckDNS, No-IP, o do roteador…).
5. No painel, **Rede → Acesso externo**: habilite, informe o IP público ou o
   nome DNS e salve (o servidor de mídia reinicia).
6. Gere um **novo QR Code** e pareie (ou repareie) os celulares.
7. No celular, **desligue o Wi-Fi** e use *Testar conexão externa*. O painel
   registra "Acesso externo confirmado".

## Segurança

- Somente as portas acima ficam expostas; o painel exige senha e escuta só
  em 127.0.0.1; o obs-websocket nunca é exposto.
- Aparelhos usam credencial própria (revogável) e TLS com pinning.
- Tentativas inválidas bloqueiam o IP temporariamente.
- Mantenha o Windows atualizado. No Windows 10 (fora de suporte), evite
  expor o computador à internet.

## Qualidade em redes móveis

O app reduz automaticamente bitrate, resolução e FPS conforme a banda
disponível, a perda de pacotes, o aquecimento e a bateria, limitando a
2,5 Mbps em rede móvel. Na troca Wi-Fi ↔ 4G/5G uma nova sessão é aberta
automaticamente (o OBS reconecta sozinho). Metas iniciais: até 2 s de
latência em boas condições — a validar em campo.

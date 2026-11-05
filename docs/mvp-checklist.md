# Checklist do MVP (especificação §20)

Legenda de status:
- ⬜ não iniciado
- 🟨 implementado (código existe, sem validação)
- 🟩 testado automaticamente (teste no repositório, executado no CI/dev)
- ✅ validado manualmente (registro em `docs/test-reports/`)

| # | Critério | Status | Como validar |
|---|---|---|---|
| 1 | Instalação por `.exe` no Windows | ⬜ | Executar `SacraLink-Setup-x64.exe` em VM Windows 10 e 11 limpas |
| 2 | Serviços iniciam sem Docker/terminal | ⬜ | Após instalar, abrir pelo atalho; verificar bandeja e `mediamtx.exe` no Gerenciador de Tarefas |
| 3 | App mostra estado de funcionamento | ⬜ | Cabeçalho do painel: servidor, OBS, câmeras |
| 4 | Android transmite vídeo real por Wi-Fi | ⬜ | Parear e iniciar transmissão; card da câmera mostra vídeo |
| 5 | iPhone transmite vídeo real por Wi-Fi | ⬜ | Idem com iPhone |
| 6 | Servidor recebe os fluxos | ⬜ | Card mostra STREAMING com bitrate > 0 |
| 7 | Vídeo reproduzido no OBS em fonte utilizável | ⬜ | Fonte `SacraLink — Câmera 01` exibe imagem |
| 8 | Áudio do dispositivo recebido e controlável | ⬜ | Medidor no mixer do OBS e no painel; mutar pelo painel |
| 9 | Fonte cadastrada/associada automaticamente | ⬜ | Fonte e cena criadas sem ação manual; reconexão não duplica |
| 10 | Conexão OBS autenticada | ⬜ | Senha errada → erro claro; senha certa → conectado |
| 11 | Painel identifica câmera conectada | ⬜ | Card aparece com nome/bateria/resolução |
| 12 | Painel coloca câmera em Preview/Program | ⬜ | Botões Preview e Corte/Transição |
| 13 | Celular recebe indicação correta de NO AR | ⬜ | Trocar cena no OBS manualmente → celular atualiza em < 500 ms |
| 14 | Zoom remoto em aparelho compatível | ⬜ | Slider no painel → zoom muda e celular confirma |
| 15 | Logs e tratamento de falhas | ⬜ | `%LOCALAPPDATA%\SacraLink\logs`; matar `mediamtx.exe` → reinício automático |
| 16 | Documentação de instalação e teste | ⬜ | `docs/installation/` e `docs/testing.md` |

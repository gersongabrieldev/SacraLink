# ADR-0005 — Instalador com Inno Setup 6

- Status: Aceito
- Data: 2026-11-05

## Opções
- **Inno Setup 6**: gera `.exe` com assistente, páginas customizadas em Pascal
  Script, desinstalador, atualização in-place, compressão LZMA2, assinatura via
  `SignTool`. Instalável no CI (`choco install innosetup`).
- **WiX Toolset**: gera MSI, ideal para implantação corporativa (GPO/Intune),
  curva maior; um `.exe` exigiria bundle Burn adicional.

## Decisão
Inno Setup 6. O público-alvo (paróquias) instala manualmente; a especificação pede
`SacraLink-Setup-x64.exe` com assistente. Se surgir demanda corporativa, um MSI
pode ser adicionado sem alterar a aplicação.

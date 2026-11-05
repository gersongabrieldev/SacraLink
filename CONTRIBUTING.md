# Como contribuir com o SacraLink

Obrigado por ajudar! O SacraLink é usado durante celebrações ao vivo, então
**confiabilidade vem antes de funcionalidades**.

## Regras básicas

- Mantenha o projeto compilando e os testes passando em todo commit.
- Nada de comportamento falso/simulado apresentado como concluído. Se algo
  depende de hardware ou de biblioteca não comprovada, documente o limite.
- Não adicione dependências de nuvem/VPS/relay para o transporte de mídia.
- Nunca altere ou apague cenas/fontes do OBS que não foram criadas pelo SacraLink.
- Mudanças sensíveis à segurança (autenticação, pareamento, TLS, firewall)
  precisam de testes.
- Documentação, comentários e mensagens de commit em português do Brasil.

## Ambiente de desenvolvimento

Veja [docs/development.md](docs/development.md).

## Padrão de commits

[Conventional Commits](https://www.conventionalcommits.org/pt-br/), com descrição
em português: `feat(server): ...`, `fix(obs): ...`, `docs: ...`,
`test(tally): ...`, `ci: ...`.

## Pull requests

1. Faça um fork e crie um branch a partir do `main`.
2. Rode `go test ./...` em `apps/server`, `npm run build` em
   `apps/desktop/frontend` e `flutter analyze && flutter test` em `apps/mobile`.
3. Descreva o que foi testado manualmente (aparelhos, versão do OBS, rede).

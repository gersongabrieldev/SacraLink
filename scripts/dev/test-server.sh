#!/usr/bin/env bash
# Roda os testes do servidor Go pacote a pacote.
#
# Em máquinas com Smart App Control (Windows 11), binários de teste recém
# compilados podem ser bloqueados de forma intermitente ("An Application
# Control policy has blocked this file"). Este script repete SOMENTE esses
# casos (até 4 tentativas); falhas reais de teste falham imediatamente.
#
# Uso: scripts/dev/test-server.sh [padrão-de-pacotes] [flags extras do go test]
set -uo pipefail

cd "$(dirname "$0")/../../apps/server"
export GOTOOLCHAIN=local
export GOTMPDIR="${GOTMPDIR:-$HOME/.gotmp}"
mkdir -p "$GOTMPDIR"
if [ -d "$HOME/sdk/go/bin" ]; then export PATH="$HOME/sdk/go/bin:$PATH"; fi

pattern="${1:-./...}"
shift || true
failed=0
for pkg in $(go list "$pattern"); do
  for attempt in 1 2 3 4; do
    out=$(go test -count=1 "$@" "$pkg" 2>&1)
    code=$?
    if [ $code -ne 0 ] && grep -q "Application Control policy has blocked" <<<"$out"; then
      echo "… $pkg bloqueado pelo Smart App Control (tentativa $attempt), repetindo"
      continue
    fi
    break
  done
  if [ $code -ne 0 ]; then
    echo "$out" | grep -v '^20[0-9][0-9]/' | tail -40
    failed=1
  else
    echo "$out" | tail -1
  fi
done
exit $failed

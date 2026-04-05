#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BIN_DIR="${HOME}/.local/bin"
TARGET="${BIN_DIR}/odrys"
SOURCE="${ROOT_DIR}/bin/odrys"
SHELL_RC="${HOME}/.bashrc"
LOCAL_GO_BIN="${HOME}/.local/go/bin"

mkdir -p "${BIN_DIR}"
chmod +x "${SOURCE}"
ln -sf "${SOURCE}" "${TARGET}"

case ":${PATH}:" in
  *":${BIN_DIR}:"*)
    ADDED_PATH="already-present"
    ;;
  *)
    ADDED_PATH="pending-shell-reload"
    if ! grep -Fq 'export PATH="$HOME/.local/bin:$PATH"' "${SHELL_RC}" 2>/dev/null; then
      printf '\nexport PATH="$HOME/.local/bin:$PATH"\n' >> "${SHELL_RC}"
    fi
    ;;
esac

case ":${PATH}:" in
  *":${LOCAL_GO_BIN}:"*)
    GO_PATH_STATE="already-present"
    ;;
  *)
    GO_PATH_STATE="pending-shell-reload"
    if [ -x "${LOCAL_GO_BIN}/go" ] && ! grep -Fq 'export PATH="$HOME/.local/go/bin:$PATH"' "${SHELL_RC}" 2>/dev/null; then
      printf 'export PATH="$HOME/.local/go/bin:$PATH"\n' >> "${SHELL_RC}"
    fi
    ;;
esac

cat <<EOF
Odrys instalado localmente.

- Enlace creado: ${TARGET}
- Ejecutable origen: ${SOURCE}
- Estado del PATH: ${ADDED_PATH}
- Go local detectado: $( [ -x "${LOCAL_GO_BIN}/go" ] && printf yes || printf no )
- PATH para Go local: ${GO_PATH_STATE}

Prueba ahora:
  source ~/.bashrc
  hash -r
  odrys

O sin instalar nada:
  ./odrys
EOF

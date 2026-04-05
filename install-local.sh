#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BIN_DIR="${HOME}/.local/bin"
TARGET="${BIN_DIR}/odrys"
SOURCE="${ROOT_DIR}/bin/odrys"
SHELL_RC="${HOME}/.bashrc"

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

cat <<EOF
Odrys instalado localmente.

- Enlace creado: ${TARGET}
- Ejecutable origen: ${SOURCE}
- Estado del PATH: ${ADDED_PATH}

Prueba ahora:
  source ~/.bashrc
  hash -r
  odrys

O sin instalar nada:
  ./odrys
EOF

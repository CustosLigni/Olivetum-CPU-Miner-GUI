#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DIST_DIR="${ROOT_DIR}/dist"
APPDIR="${DIST_DIR}/OlivetumMiner.AppDir"

ETHMINER_SRC="${ETHMINER_SRC:-}"
if [[ -z "${ETHMINER_SRC}" ]]; then
  for candidate in \
    "${ROOT_DIR}/../Olivetum-GPU-Miner/build_gcc12/ethminer/ethminer" \
    "${ROOT_DIR}/../Olivetum-GPU-Miner/build/ethminer/ethminer" \
    "${ROOT_DIR}/../Olivetum-GPU-Miner-main/build/ethminer/ethminer" \
    "${ROOT_DIR}/ethminer"; do
    if [[ -x "${candidate}" ]]; then
      ETHMINER_SRC="${candidate}"
      break
    fi
  done
fi

mkdir -p "${DIST_DIR}"

if [[ ! -x "${ETHMINER_SRC}" ]]; then
  echo "ERROR: ethminer binary not found at: ${ETHMINER_SRC}" >&2
  echo "Build it first in Olivetum-GPU-Miner (or adjust ETHMINER_SRC)." >&2
  exit 1
fi

echo "Using ethminer: ${ETHMINER_SRC}"

echo "[1/4] Building GUI..."
cd "${ROOT_DIR}"
go mod tidy
go build -trimpath -ldflags="-s -w" -o "${DIST_DIR}/olivetum-miner-gui" ./...

echo "[2/4] Building AppDir..."
rm -rf "${APPDIR}"
mkdir -p "${APPDIR}/usr/bin" \
  "${APPDIR}/usr/share/applications" \
  "${APPDIR}/usr/share/icons/hicolor/scalable/apps"

cp -f "${DIST_DIR}/olivetum-miner-gui" "${APPDIR}/usr/bin/olivetum-miner-gui"
cp -f "${ETHMINER_SRC}" "${APPDIR}/usr/bin/ethminer"
chmod +x "${APPDIR}/usr/bin/olivetum-miner-gui" "${APPDIR}/usr/bin/ethminer"

cat > "${APPDIR}/usr/share/applications/olivetum-miner-gui.desktop" <<'EOF'
[Desktop Entry]
Type=Application
Name=Olivetum Miner
Comment=Simple GUI wrapper for ethminer (Olivetumhash)
Exec=olivetum-miner-gui
Icon=olivetum-miner-gui
Terminal=false
Categories=Utility;
EOF

cat > "${APPDIR}/usr/share/icons/hicolor/scalable/apps/olivetum-miner-gui.svg" <<'EOF'
<svg xmlns="http://www.w3.org/2000/svg" width="256" height="256" viewBox="0 0 256 256">
  <defs>
    <linearGradient id="g" x1="0" x2="1" y1="0" y2="1">
      <stop offset="0" stop-color="#10b981"/>
      <stop offset="1" stop-color="#0ea5e9"/>
    </linearGradient>
  </defs>
  <rect x="16" y="16" width="224" height="224" rx="48" fill="url(#g)"/>
  <path d="M128 56c-39.8 0-72 32.2-72 72s32.2 72 72 72 72-32.2 72-72-32.2-72-72-72zm0 28c24.3 0 44 19.7 44 44s-19.7 44-44 44-44-19.7-44-44 19.7-44 44-44z" fill="#0b1220" opacity="0.85"/>
  <path d="M88 128c0-22.1 17.9-40 40-40v24c-8.8 0-16 7.2-16 16s7.2 16 16 16v24c-22.1 0-40-17.9-40-40z" fill="#ffffff" opacity="0.9"/>
</svg>
EOF

cat > "${APPDIR}/AppRun" <<'EOF'
#!/usr/bin/env bash
HERE="$(dirname "$(readlink -f "$0")")"
export PATH="${HERE}/usr/bin:${PATH}"
exec "${HERE}/usr/bin/olivetum-miner-gui" "$@"
EOF
chmod +x "${APPDIR}/AppRun"

# AppImage expects the desktop file + icon in AppDir root too.
cp -f "${APPDIR}/usr/share/applications/olivetum-miner-gui.desktop" "${APPDIR}/olivetum-miner-gui.desktop"
cp -f "${APPDIR}/usr/share/icons/hicolor/scalable/apps/olivetum-miner-gui.svg" "${APPDIR}/olivetum-miner-gui.svg"

echo "[3/4] Fetching appimagetool..."
APPIMAGETOOL="${DIST_DIR}/appimagetool-x86_64.AppImage"
if [[ ! -x "${APPIMAGETOOL}" ]]; then
  curl -L -o "${APPIMAGETOOL}" "https://github.com/AppImage/AppImageKit/releases/download/continuous/appimagetool-x86_64.AppImage"
  chmod +x "${APPIMAGETOOL}"
fi

echo "[4/4] Creating AppImage..."
OUT="${DIST_DIR}/OlivetumMiner-x86_64.AppImage"
"${APPIMAGETOOL}" "${APPDIR}" "${OUT}"

echo "Done: ${OUT}"

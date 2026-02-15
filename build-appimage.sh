#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DIST_DIR="${ROOT_DIR}/dist"
APPDIR="${DIST_DIR}/OlivetumMiner.AppDir"

export LC_ALL=C
export TZ=UTC

if [[ -z "${SOURCE_DATE_EPOCH:-}" ]]; then
  if command -v git >/dev/null 2>&1 && git -C "${ROOT_DIR}" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    SOURCE_DATE_EPOCH="$(git -C "${ROOT_DIR}" show -s --format=%ct HEAD 2>/dev/null || true)"
  fi
  if [[ -z "${SOURCE_DATE_EPOCH:-}" ]]; then
    SOURCE_DATE_EPOCH="$(date -u +%s)"
  fi
fi

APPIMAGETOOL_SHA256="b90f4a8b18967545fda78a445b27680a1642f1ef9488ced28b65398f2be7add2"

XMRIG_SRC="${XMRIG_SRC:-}"
if [[ -z "${XMRIG_SRC}" ]]; then
  for candidate in \
    "${ROOT_DIR}/../xmrig-olivetum/build/xmrig" \
    "${ROOT_DIR}/../xmrig-olivetum/bin/xmrig" \
    "${ROOT_DIR}/xmrig"; do
    if [[ -x "${candidate}" ]]; then
      XMRIG_SRC="${candidate}"
      break
    fi
  done
fi

GETH_SRC="${GETH_SRC:-}"
if [[ -z "${GETH_SRC}" ]]; then
  for candidate in \
    "${ROOT_DIR}/../core-geth/build/bin/geth" \
    "${ROOT_DIR}/geth"; do
    if [[ -x "${candidate}" ]]; then
      GETH_SRC="${candidate}"
      break
    fi
  done
fi

GENESIS_SRC="${GENESIS_SRC:-${ROOT_DIR}/assets/olivetum_pow_genesis.json}"

mkdir -p "${DIST_DIR}"

if [[ ! -x "${XMRIG_SRC}" ]]; then
  echo "ERROR: xmrig binary not found at: ${XMRIG_SRC}" >&2
  echo "Build it first in xmrig-olivetum (or adjust XMRIG_SRC)." >&2
  exit 1
fi

if [[ ! -x "${GETH_SRC}" ]]; then
  echo "ERROR: geth binary not found at: ${GETH_SRC}" >&2
  echo "Build it first in core-geth (or adjust GETH_SRC)." >&2
  exit 1
fi

if [[ ! -f "${GENESIS_SRC}" ]]; then
  echo "ERROR: genesis file not found at: ${GENESIS_SRC}" >&2
  exit 1
fi

echo "Using xmrig: ${XMRIG_SRC}"
echo "Using geth: ${GETH_SRC}"

echo "[1/4] Building GUI..."
cd "${ROOT_DIR}"
go mod download
go build -trimpath -buildvcs=false -ldflags="-s -w -buildid=" -o "${DIST_DIR}/olivetum-miner-gui" ./...

echo "[2/4] Building AppDir..."
rm -rf "${APPDIR}"
mkdir -p "${APPDIR}/usr/bin" \
  "${APPDIR}/usr/share/olivetum" \
  "${APPDIR}/usr/share/applications" \
  "${APPDIR}/usr/share/icons/hicolor/scalable/apps"

cp -f "${DIST_DIR}/olivetum-miner-gui" "${APPDIR}/usr/bin/olivetum-miner-gui"
cp -f "${XMRIG_SRC}" "${APPDIR}/usr/bin/xmrig"
cp -f "${GETH_SRC}" "${APPDIR}/usr/bin/geth"
cp -f "${GENESIS_SRC}" "${APPDIR}/usr/share/olivetum/olivetum_pow_genesis.json"
chmod +x "${APPDIR}/usr/bin/olivetum-miner-gui" "${APPDIR}/usr/bin/xmrig" "${APPDIR}/usr/bin/geth"

cat > "${APPDIR}/usr/share/applications/olivetum-miner-gui.desktop" <<'EOF'
[Desktop Entry]
Type=Application
Name=Olivetum Miner
Comment=Simple GUI wrapper for XMRig RandomX (Olivetum)
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

if command -v touch >/dev/null 2>&1; then
  if ! find "${APPDIR}" -exec touch -h -d "@${SOURCE_DATE_EPOCH}" {} + >/dev/null 2>&1; then
    find "${APPDIR}" -exec touch -d "@${SOURCE_DATE_EPOCH}" {} +
  fi
fi

echo "[3/4] Fetching appimagetool..."
APPIMAGETOOL="${DIST_DIR}/appimagetool-x86_64.AppImage"
if [[ ! -x "${APPIMAGETOOL}" ]]; then
  curl -L -o "${APPIMAGETOOL}" "https://github.com/AppImage/AppImageKit/releases/download/continuous/appimagetool-x86_64.AppImage"
  chmod +x "${APPIMAGETOOL}"
fi
if command -v sha256sum >/dev/null 2>&1; then
  actual="$(sha256sum "${APPIMAGETOOL}" | awk '{print $1}')"
  if [[ "${actual}" != "${APPIMAGETOOL_SHA256}" ]]; then
    echo "ERROR: appimagetool SHA256 mismatch: ${actual}" >&2
    echo "Expected: ${APPIMAGETOOL_SHA256}" >&2
    exit 1
  fi
fi

echo "[4/4] Creating AppImage..."
OUT="${DIST_DIR}/OlivetumMiner-x86_64.AppImage"
OUT_TMP="${OUT}.tmp.$$"
MKSQUASHFS_BIN="$(command -v mksquashfs || true)"
if [[ -z "${MKSQUASHFS_BIN}" ]]; then
  echo "ERROR: mksquashfs not found. Install squashfs-tools." >&2
  exit 1
fi

TOOL_TMP="$(mktemp -d)"
cleanup() {
  rm -rf "${TOOL_TMP}" || true
}
trap cleanup EXIT

(cd "${TOOL_TMP}" && "${APPIMAGETOOL}" --appimage-extract >/dev/null)
rm -f "${TOOL_TMP}/squashfs-root/usr/lib/appimagekit/mksquashfs"
ln -s "${MKSQUASHFS_BIN}" "${TOOL_TMP}/squashfs-root/usr/lib/appimagekit/mksquashfs"

env -u SOURCE_DATE_EPOCH "${TOOL_TMP}/squashfs-root/AppRun" \
  --mksquashfs-opt "-processors" --mksquashfs-opt "1" \
  --mksquashfs-opt "-mkfs-time" --mksquashfs-opt "${SOURCE_DATE_EPOCH}" \
  --mksquashfs-opt "-all-time" --mksquashfs-opt "${SOURCE_DATE_EPOCH}" \
  "${APPDIR}" "${OUT_TMP}"
mv -f "${OUT_TMP}" "${OUT}"

echo "Done: ${OUT}"

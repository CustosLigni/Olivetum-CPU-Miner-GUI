# Olivetum Miner GUI

GUI wrapper for `xmrig` (RandomX / `rx/olivetum`) with optional embedded `geth` node.

## Features

- CPU mining (XMRig) for Olivetum RandomX
- Stratum mode and daemon RPC modes (`daemon+http(s)://`)
- Optional embedded node (geth) with bundled genesis (AppImage)
- CPU thread selection, thread count, huge pages, MSR options
- Dashboard with hashrate history, per-CPU table, logs
- AppImage packaging for Linux x86_64

## Requirements

- Go 1.22+
- Linux build dependencies for Fyne (OpenGL + X11): https://developer.fyne.io/started/

## Build (binary)

```bash
mkdir -p dist
go mod tidy
go build -trimpath -ldflags="-s -w" -o dist/olivetum-miner-gui .
```

`xmrig` should be next to the GUI binary or available in `PATH`.
For embedded node support outside AppImage, `geth` should also be next to the GUI binary or in `PATH`.

## Build (Windows)

Fyne uses GLFW (cgo), so a working C toolchain is required (MSYS2/MinGW or Visual Studio Build Tools).

```powershell
mkdir dist
go mod tidy
go build -trimpath -ldflags="-H=windowsgui -s -w" -o dist\\OlivetumMiner.exe .
```

Place `xmrig.exe` next to `OlivetumMiner.exe` (or in `PATH`).
For embedded node support, place `geth.exe` next to `OlivetumMiner.exe` (or in `PATH`).

## Build (AppImage)

The AppImage bundles the GUI, `xmrig`, `geth` and the Olivetum genesis.

```bash
export XMRIG_SRC=/path/to/xmrig
export GETH_SRC=/path/to/geth
./build-appimage.sh
```

Output:

```text
dist/OlivetumMiner-x86_64.AppImage
```

If `XMRIG_SRC`/`GETH_SRC` are not set, the script attempts auto-detection from sibling repos.

## Run (AppImage)

```bash
chmod +x OlivetumMiner-x86_64.AppImage
./OlivetumMiner-x86_64.AppImage
```

On some distros you may need FUSE (`libfuse2`/`fuse`) for AppImages.

## Configuration

User settings are stored in:

```text
~/.config/olivetum-miner-gui/config.json
```

## Embedded node (geth)

In `Setup` you can enable `Run a node` and start/stop the node from GUI.

For local daemon mining:

1. Select `Mode` -> `Solo (Local RPC)`
2. Enable `Run a node`
3. Set node mining address in Node settings (if needed)
4. Start node, then start mining

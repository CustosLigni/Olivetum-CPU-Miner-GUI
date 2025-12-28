# Olivetum Miner GUI

A modern GUI wrapper for `ethminer` with Olivetumhash support. Built with Fyne.

## Features

- Quick Start with mining mode selection (Stratum / Solo RPC)
- GPU backend selector (Auto / CUDA / OpenCL)
- Per-device selection and live stats
- Dashboard with hashrate history and logs
- AppImage packaging for Linux x86_64

## Requirements

- Go 1.22+
- Linux build dependencies for Fyne (OpenGL + X11). See:
  https://developer.fyne.io/started/

## Build (binary)

```bash
mkdir -p dist
go mod tidy
go build -o dist/olivetum-miner-gui .
```

`ethminer` must be in the same directory as the GUI binary or available in `PATH`.

## Getting `ethminer`

This project is a GUI wrapper, it does not build `ethminer` from source.

- Build `ethminer` from the Olivetum fork/repo, then point the GUI to it (same directory or `PATH`).
- For AppImage packaging, provide the built `ethminer` path via `ETHMINER_SRC` (see below).

## Build (AppImage)

The AppImage bundles the GUI and `ethminer`.

```bash
export ETHMINER_SRC=/path/to/ethminer
./build-appimage.sh
```

The script downloads `appimagetool` if missing and produces:
`dist/OlivetumMiner-x86_64.AppImage`

## Run (AppImage)

```bash
chmod +x OlivetumMiner-x86_64.AppImage
./OlivetumMiner-x86_64.AppImage
```

On some distros you may need FUSE (`libfuse2`/`fuse`) to run AppImages.

## Configuration

User settings are stored locally in:

```
~/.config/olivetum-miner-gui/config.json
```

This file is not part of the repository and is created on first run.

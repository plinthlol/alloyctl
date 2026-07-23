<div align="center">

<img src="logo.svg" width="200" />

# alloyctl

**A clean, fast Minecraft launcher**

---

</div>

## Features

- **Multi-loader support** — Vanilla, Fabric, Quilt, Forge, NeoForge
- **Smart Java detection** — Auto-finds your Java install, or set your own
- **Instance management** — Multiple profiles, each with their own mods/saves
- **Cross-platform** — Linux, macOS, Windows
- **Clean terminal UX** — Launch from terminal, close it, game keeps running

## Install

```bash
go install github.com/plinthlol/alloyctl@latest
```

Or download a release from [Releases](https://github.com/plinthlol/alloyctl/releases).

## Usage

```bash
# Authenticate
alloyctl auth online        # Microsoft account
alloyctl auth offline steve # Offline/cracked

# Install
alloyctl install 1.20.4                    # Vanilla
alloyctl install 1.20.4 --fabric          # Fabric
alloyctl install 1.20.4 --forge           # Forge
alloyctl install 1.20.4 --quilt           # Quilt
alloyctl install 1.20.4 --neoforge        # NeoForge

# Play
alloyctl play                  # List installed instances
alloyctl play 1.20.4-fabric   # Launch specific instance

# Manage
alloyctl rename old new
alloyctl remove 1.20.4-fabric

# Java
alloyctl java list
alloyctl java set /path/to/java
```

## Launch Options

```bash
alloyctl play [instance] \
  --memory 4096 \
  --width 1920 --height 1080 \
  --jvm "-Xss4m"
```

## Config

Alloy stores everything in standard XDG directories:

| Purpose | Linux | macOS | Windows |
|---------|-------|-------|---------|
| Config | `~/.config/alloy/` | `~/Library/Application Support/alloy/` | `%APPDATA%/alloy/` |
| Data | `~/.local/share/alloy/` | `~/Library/Application Support/alloy/` | `%LOCALAPPDATA%/alloy/` |
| Cache | `~/.cache/alloy/` | `~/Library/Caches/alloy/` | `%LOCALAPPDATA%/alloy/cache/` |

## License

MIT

<div align="center">

<img src="logo.svg" width="200" />

# alloyctl

**A Minecraft launcher. Minimal but featureful.**

---

</div>

## Install

**One-liner:**

```bash
curl -sL https://raw.githubusercontent.com/plinthlol/alloyctl/master/install.sh | sh
```

This auto-detects your platform (Linux (GNU, musl, etc.), macOS, Windows).

**Or manually:**

Download from [Releases](https://github.com/plinthlol/alloyctl/releases) and place in your PATH.

**Or build from source:**

```bash
go install github.com/plinthlol/alloyctl@latest
```

## Commands

### Auth

```bash
alloyctl auth online              # Microsoft account (OAuth device flow)
alloyctl auth offline <username>  # Offline/cracked account
```

### Install

```bash
alloyctl install <version>                        # Vanilla
alloyctl install <version> --fabric              # Fabric
alloyctl install <version> --quilt               # Quilt
alloyctl install <version> --forge               # Forge
alloyctl install <version> --neoforge            # NeoForge
alloyctl install <version> --name my-server      # Custom instance name
```

### Play

```bash
alloyctl play                                      # List installed instances
alloyctl play <instance>                           # Launch instance
alloyctl play <instance> --memory 4096             # Custom memory (MB)
alloyctl play <instance> --width 1920 --height 1080  # Custom resolution
alloyctl play <instance> --jvm "-Xss4m"            # Custom JVM arg (repeatable)
```

### Manage

```bash
alloyctl rename <old> <new>    # Rename instance
alloyctl remove <instance>     # Remove instance (preserves shared cache)
```

### Java

```bash
alloyctl java list             # List detected Java installations
alloyctl java set <path>       # Set global Java override
```

## Configuration

Alloy stores everything in standard XDG directories:

| Platform | Config | Data | Cache |
|----------|--------|------|-------|
| Linux | `~/.config/alloy/` | `~/.local/share/alloy/` | `~/.cache/alloy/` |
| macOS | `~/Library/Application Support/alloy/` | `~/Library/Application Support/alloy/` | `~/Library/Caches/alloy/` |
| Windows | `%APPDATA%/alloy/` | `%LOCALAPPDATA%/alloy/` | `%LOCALAPPDATA%/alloy/cache/` |

### File Layout

```
~/.config/alloy/
├── config.toml                    # Global settings (accounts, java, defaults)
└── instances/
    └── <name>/
        └── instance.toml          # Per-instance metadata

~/.local/share/alloy/
├── instances/
│   └── <name>/                    # Mutable game directory
│       ├── mods/
│       ├── saves/
│       ├── resourcepacks/
│       ├── config/
│       └── logs/
└── cache/
    ├── versions/
    │   └── <key>/                 # Shared immutable version cache
    │       ├── version.json
    │       ├── client.jar
    │       └── libraries/
    └── assets/                    # Shared assets (textures, sounds)
```

### config.toml

```toml
active_account = "Steve"
java_path = "/usr/lib/jvm/java-21/bin/java"
default_memory_mb = 4096

[[accounts]]
type = "microsoft"
username = "Steve"
uuid = "..."
refresh_token = "..."
```

### instance.toml

```toml
name = "1.20.4-fabric"
mc_version = "1.20.4"
loader = "fabric"
loader_version = "0.15.11"
java_path = "/custom/java"     # Optional per-instance override
memory_mb = 8192               # Optional per-instance override
jvm_args = ["-Xss4m"]          # Optional persistent JVM args
```

## License

MIT

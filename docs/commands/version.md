# version Command

The `version` command displays version information for morphic, including the current version number, build date, git commit, and platform details.

## Usage

```
morphic version [flags]
```

## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--format`, `-f` | string | `text` | Output format: `text` or `json` |
| `--build-info`, `-b` | bool | `false` | Show detailed build information |

## Global Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--config` | string | `migrations/morphic.config.yaml` | Path to the configuration file |

---

## Examples

### Show version

```bash
morphic version

# Output:
# morphic v1.4.2
```

### Show detailed build info

```bash
morphic version --build-info

# Output:
# morphic v1.4.2
# Build Date: 2026-03-20
# Git Commit: 0a556ef
# Go Version: go1.24
# Platform: linux/amd64
```

### JSON output (useful for CI/CD)

```bash
morphic version --format json

# Output:
# {"version":"1.4.2","build_date":"2026-03-20","git_commit":"0a556ef","go_version":"go1.24","platform":"linux/amd64"}
```

---

## See Also

- [init command](./init.md) — Initialise the migrations directory
- [morphic command](./morphic.md) — Generate migrations from schema changes

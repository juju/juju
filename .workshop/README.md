# Workshop Development Environment

This directory defines the workshop development environment for Juju. Workshop
provisions reproducible, containerised workspaces with project-specific SDKs and
tooling pre-installed.

## Directory Structure

```
.workshop/
├── dev-lxd.yaml          # Primary workspace definition (with LXD cloud)
├── Makefile              # Convenience targets for common workflows
├── dqlite/              
│   ├── sdk.yaml          # SDK descriptor (name: dqlite)
│   └── hooks/
│       ├── setup-base    # Installs base OS packages (runs as root)
│       ├── setup-project # Downloads musl + dqlite static libs
│       └── check-health  # Verifies musl-gcc and dqlite are present
├── juju/
│   ├── sdk.yaml          # SDK descriptor (name: juju)
│   └── hooks/
│       ├── setup-project # Builds juju CLI and jujud-controller binaries
│       └── check-health  # Verifies built binaries are present
├── lxd-cloud/
│   ├── sdk.yaml          # SDK descriptor (name: lxd-cloud, privileged setup-base)
│   └── hooks/
│       ├── setup-base    # Installs LXD client dependencies (runs as root)
│       ├── setup-project # Generates client certs, discovers host LXD, writes juju credentials
│       └── check-health  # Reports LXD connectivity status
└── toolchain/
    ├── sdk.yaml          # SDK descriptor (name: toolchain)
    └── hooks/
        ├── setup-base    # Writes /etc/profile.d/gotoolchain.sh (runs as root)
        └── setup-project # Configures GOTOOLCHAIN via `go env -w`
```

## Workspace Definition (`dev-lxd.yaml`)

The `dev-lxd.yaml` file declares a workspace named `dev-lxd` built on Ubuntu
24.04. It composes several SDKs and exposes actions that can be invoked via
`workshop run dev-lxd <action>`.

### SDKs

| SDK                  | Purpose                                            |
|----------------------|----------------------------------------------------|
| `go`                 | Go toolchain (channel: 1.26/stable)                |
| `project-toolchain`  | Pins GOTOOLCHAIN so `go.mod` versions auto-fetch   |
| `project-dqlite`     | Builds musl and dqlite static libraries            |
| `project-lxd-cloud`  | Generates LXD client certs and juju credentials    |
| `opencode`           | Installs the opencode development tool             |
| `direnvrc`           | Provides direnv integration                        |
| `project-juju`       | Builds juju and jujud-controller (runs last)       |

SDKs prefixed with `project-` reference the local SDK definitions in this
directory (e.g. `project-dqlite` maps to `./dqlite/`).

The `project-juju` SDK is listed last in `dev-lxd.yaml` so that all build
dependencies (Go toolchain, dqlite static libs) are ready before compilation.

**Note:** The `project-lxd-cloud` SDK does not install or configure LXD itself.
It only sets up client credentials for communicating with a pre-existing host
LXD instance over HTTPS. See [Bootstrap LXD](#bootstrap-lxd) for details.

### Actions

| Action             | Description                                |
|--------------------|--------------------------------------------|
| `opencode`         | Launches opencode within a direnv context  |
| `test`             | Runs `make run-tests` with passed args     |
| `build-juju`       | Builds the juju CLI binary                 |
| `build-controller` | Builds the jujud-controller binary         |

## SDK Hook Lifecycle

Each SDK directory contains a `sdk.yaml` descriptor and a `hooks/` directory.
Hooks are shell scripts executed at specific lifecycle stages:

1. **`setup-base`** — Runs once during image build (typically as root). Installs
   OS packages and system-level configuration.
2. **`setup-project`** — Runs after the project is mounted. Downloads
   dependencies or configures project-specific state.
3. **`check-health`** — Runs periodically to report SDK readiness. Must call
   `workshopctl set-health okay` on success or
   `workshopctl set-health --code="<code>" error "<message>"` on failure.

Not all hooks are required — only provide those needed by the SDK.

### Privileged Hooks

If a hook requires elevated privileges (e.g. installing packages), declare it
in `sdk.yaml`:

```yaml
hooks:
  setup-base:
    privileged: true
```

## Makefile Targets

The `.workshop/Makefile` provides targets invoked from the host:

- **`workshop-dev-lxd-trust`** — Exports the workshop client certificate and
  adds it to the host LXD trust store, enabling the workspace to communicate
  with host LXD over HTTPS.

## Typical Workflow

```bash
# Launch the dev-lxd workspace (builds juju + controller automatically via project-juju SDK)
workshop launch dev-lxd

# Rebuild juju after making changes
workshop run dev-lxd build-juju

# Run tests
workshop run dev-lxd test ./domain/...
```

## Bootstrap LXD

Workshop containers cannot run LXD themselves — nested LXD requires a VM or
privileged container with significant kernel feature exposure. Until workshop
gains native VM support, the workaround is to use the **host's LXD** as the
bootstrap substrate. The workshop container communicates with host LXD over
HTTPS using a generated client certificate.

### Architecture

```
┌──────────────────────────────────┐
│  Host                            │
│  ┌───────────┐                   │
│  │ LXD :8443 │◄─── HTTPS ───┐   │
│  └───────────┘               │   │
│                              │   │
│  ┌───────────────────────────┼─┐ │
│  │ Workshop container        │ │ │
│  │                           │ │ │
│  │  juju bootstrap lxd ─────┘ │ │
│  │  (uses client cert + host   │ │
│  │   IP from default route)    │ │
│  └─────────────────────────────┘ │
└──────────────────────────────────┘
```

### How It Works

1. **`setup-project` (automatic)** — On workspace start, the LXD cloud SDK
   generates a self-signed TLS client certificate at
   `~/.config/lxc/client.crt`, discovers the host LXD address via the
   container's default gateway, fetches the LXD server certificate over TLS,
   and writes both `clouds.yaml` and `credentials.yaml` into
   `~/.local/share/juju/`. This runs automatically — no manual priming step is
   needed.

2. **`workshop-dev-lxd-trust` (host-side, manual)** — The host must trust the
   workshop's client certificate. This Makefile target extracts the certificate
   from the container and adds it to the host LXD trust store. Run this once
   (or after certificate regeneration).

### Prerequisites

The host LXD must be listening on an HTTPS address accessible from the
container network:

```bash
lxc config set core.https_address "[::]:8443"
```

### Usage

```bash
# 1. On the host — trust the workshop certificate
make -f .workshop/Makefile workshop-dev-lxd-trust

# 2. Inside the workshop — bootstrap (credentials already configured)
workshop shell dev-lxd
juju bootstrap lxd
```

### Why Not Run LXD Inside the Container?

Workshop uses unprivileged system containers. LXD requires either:
- A VM (full kernel), or
- A privileged container with access to cgroups, device nodes, and network
  namespaces that workshop does not (and should not) expose.

Using the host LXD over HTTPS avoids these constraints entirely. Once workshop
supports VM-based workspaces, it will be possible to run LXD natively inside
the workspace and this bridge approach will become optional.

## Adding a New SDK

1. Create a directory under `.workshop/` with the SDK name.
2. Add a `sdk.yaml` with at minimum `name: <sdk-name>`.
3. Add hook scripts under `hooks/` (ensure they are executable).
4. Reference the SDK in `dev-lxd.yaml` as `project-<sdk-name>`.

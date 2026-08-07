# Spread integration tests

This directory contains Juju's integration tests written for
[spread](https://github.com/canonical/spread) (the `adhoc`/LXD backend). They
are the migration target for the legacy bash suites under `tests/suites/`.

Each subdirectory of a suite (e.g. `deploy/deploy-charm/`) is one *task*; a
`task.yaml` file defines the task's `execute` (and optional `prepare`/`restore`
steps). Tasks run against a controller brought up with the Juju binary built
from this repository.

## Prerequisites

- A Linux host with **LXD** (including VM support) and **KVM** enabled. The
  lxd backend launches an Ubuntu VM with `lxc launch --vm` and runs LXD inside
  it, so nested virtualization is required for tests that add machines.
- The `spread` binary, built from the `canonical/spread` source **with a local
  fix** in `spread/client.go` (see below). `juju`, `yq`, `charmcraft` do *not*
  need to be installed on the host — they are set up inside the VM.

### Building the patched `spread`

The upstream `SendTar` emits `tar --no-same-owner xz`, which GNU tar misparses
(the bare `xz` is treated as a file operand), breaking project delivery to the
VM. The working binary is a local build with that one line fixed:

```sh
# In the checked-out canonical/spread source (spread/spread/client.go):
#   SendTar remote command:  /bin/tar -xz --no-same-owner ...
cd <spread-source>
GOBIN=<your go bin> go install ./cmd/spread
```

## Running tests

Run from the repository root (where `spread.yaml` lives).

List the available tasks:

```sh
spread -list
```

Run a single test on one system:

```sh
spread -v lxd:ubuntu-22.04:tests/spread/deploy/deploy-charm
```

Run a few specific tests:

```sh
spread -v lxd:ubuntu-22.04:tests/spread/deploy/deploy-charm \
       lxd:ubuntu-22.04:tests/spread/deploy/deploy-default-base
```

Run an entire suite (all runnable tasks in `deploy/`, sharing one controller):

```sh
spread -v lxd:ubuntu-22.04:tests/spread/deploy
```

Run a suite on every declared system, or everything:

```sh
spread -v lxd:tests/spread/deploy     # all systems
spread -v                             # all suites, all systems
```

`-v` shows progress; use `-vv` for debug output. Add `-abend` to stop on the
first failure instead of continuing.

### Reusing the VM (important for speed)

On a fresh VM, spread builds `juju` and `jujud-controller` inside the VM
(~10–15 min). Keep a VM alive across runs to skip the rebuild:

```sh
spread -reuse -v lxd:ubuntu-22.04:tests/spread/deploy/deploy-charm
```

After changing any repository content while reusing, re-send the project:

```sh
spread -reuse -resend -v lxd:ubuntu-22.04:tests/spread/deploy/deploy-charm
```

If the reuse VM is no longer valid (deleted, or reused for a different
setup), clear the reuse metadata:

```sh
rm -f .spread-reuse*
```

A kept VM is left running after a `-reuse` run and is reused by the next one.

## Reading results and logs

By default spread prints only the *phase* summary (`Preparing`/`Executing`/
`Restoring`, then `Successful tasks: N`). On a failure it prints the failing
task's output between `----- ... -----` markers. There are two ways to actually
see what happened.

### 1. See output live while it runs

- `spread -v` — show progress and each task's output as it runs (folded).
- `spread -vv` — full trace of every script line as it runs (very verbose).
- `spread -abend` — stop on the first failure, so the output is not buried
  under subsequent tasks.

### 2. Inspect the saved logs on the runner VM (recommended)

The juju-level logs are **not** kept on the host. Each spread task sources
`tests/lib/spread-env.sh`, which sets `TEST_DIR` (the `deploy` suite sets it to
`/tmp/spread-juju-deploy`) and writes everything there:

- `test-<model>.log` — the `ensure`/bootstrap output for that model,
- `<controller>-<model>-debug.log` — a `juju debug-log` tail for that model,
- `<controller>-controller-debug.log` — the controller debug log,
- `*-destroy.log` / `*-bootstrap.log` — teardown/bootstrap captures,
- `jujus`, `models`, `pids` — bookkeeping files.

The VM must still exist to read them, so run with `-reuse` (which keeps the VM
running instead of discarding it at the end). Then:

```sh
# find the runner VM spread is using
lxc list | grep juju-ubuntu

# list the log files
lxc exec <vm> -- ls -la /tmp/spread-juju-deploy

# pull the whole log dir to the host
lxc file pull -r <vm>/tmp/spread-juju-deploy ./spread-logs

# or cat a specific log
lxc exec <vm> -- cat /tmp/spread-juju-deploy/test-deploy-charm.log
```

> Note: `spread -artifacts <dir>` only copies files a task *declares* in its
> `artifacts:` field (relative to the task directory). The juju logs live under
> `TEST_DIR` outside the task directory, so the `lxc` commands above are the way
> to fetch them.

### 3. When the VM is gone

If you ran without `-reuse`, the VM (and its `/tmp` logs) is discarded when the
run ends and the logs are lost. Use `-reuse` (or `-reuse -resend`) whenever you
may want to inspect the logs afterwards.

## Configuration

Most knobs are overridable via environment variables (see `spread.yaml`):

| Variable | Purpose | Default |
|---|---|---|
| `BOOTSTRAP_PROVIDER` | Where Juju bootstraps (`lxd`, `ec2`, `gce`, `azure`, …) | `lxd` |
| `BOOTSTRAP_CLOUD` | Cloud name for non-lxd providers | *(empty)* |
| `BOOTSTRAP_BASE` | Controller base; must be non-empty for controller reuse to work | `ubuntu@24.04` (`deploy` suite) |
| `BOOTSTRAP_ARCH` / `MODEL_ARCH` | Architecture constraints | *(empty)* |
| `BASE` | Base image of the runner VM (`noble`, `jammy`, …) | `noble` |
| `DISK` / `CPU` / `MEM` | Runner VM sizing | `40` / `8` / `16` |

Example: run the deploy suite with an explicit runner image:

```sh
BASE=jammy spread -v lxd:ubuntu-22.04:tests/spread/deploy
```

These variables are exported into the runner VM (via the `$(HOST: ...)`
expressions in `spread.yaml`), so they can be set on the host before invoking
`spread`. No spread-specific config file is needed beyond `spread.yaml`.

## Running against AWS (local proof-of-concept)

The `lxd` backend VM is only the *runner*; Juju boots the controller wherever
`BOOTSTRAP_PROVIDER` points. To run a test against AWS, set the provider and
pass the credentials on the command line:

```sh
BOOTSTRAP_PROVIDER=ec2 \
BOOTSTRAP_CLOUD=aws \
AWS_ACCESS_KEY_ID=... \
AWS_SECRET_ACCESS_KEY=... \
AWS_DEFAULT_REGION=... \
spread -reuse -v lxd:ubuntu-22.04:tests/spread/deploy/deploy-charm
```

Notes:

- The AWS-specific bash tests (bundle `image-id` variants) are **not yet
  ported** to `tests/spread/deploy/`; the above runs a generic test against a
  cloud-bootstrapped controller, which validates the non-lxd code path.
- Never commit AWS credentials. CI credentials are still to be arranged (see
  the migration plan, Step 5).

## Notes

- Spread only scans **declared** suites (the `suites:` section in `spread.yaml`)
  and treats each immediate subdirectory containing a `task.yaml` as a task. A
  new suite must be added to `spread.yaml`.
- Some tasks carry `manual: true` or provider gates (see the migration plan for
  the current rationale) — they are intentionally excluded/conditional.
- Full background and remaining work is tracked in
  [`docs/spread-test-migration-plan.md`](../../docs/spread-test-migration-plan.md).

# Developer Fixture: Local snapd bring-up for `jujud` controller snap

This directory contains the minimum developer fixture needed to exercise the
`jujud` snap service under `snapd` during local development. The fixture is a
host-side staging aid; it is not packaged into the snap payload.

## Prerequisites

The `jujud` snap must be built and installed:

```bash
make jujud-snap
sudo snap install --dangerous ./jujud_*.snap
```

## Fixture location

After `snap install`, stage the fixture under the snap's writable common
directory. The hard launch input is a valid `runtime.conf` at:

```
/var/snap/jujud/common/var/lib/juju/agents/controller-0/runtime.conf
```

Create the required directories and stage the fixture:

```bash
sudo mkdir -p /var/snap/jujud/common/var/lib/juju/agents/controller-0
sudo mkdir -p /var/snap/jujud/common/var/log/juju
sudo cp runtime.conf.example /var/snap/jujud/common/var/lib/juju/agents/controller-0/runtime.conf
sudo chmod 600 /var/snap/jujud/common/var/lib/juju/agents/controller-0/runtime.conf
```

## Acceptable fixture sources

The `runtime.conf` must contain all fields required by the controller startup
validation. Two sources are acceptable:

1. **Controller runtime data produced by the existing bootstrap machinery.**
   If you have a Juju environment that has already bootstrapped a controller,
   copy the `runtime.conf` from that controller's agent directory.

2. **A purpose-built local test fixture assembled to match the `runtime.conf`
   contract.** Use `runtime.conf.example` in this directory as a template.
   The template contains valid dummy values that pass controller startup
   validation. Replace them with real values and source TLS key material
   from bootstrap output or a test fixture before non-test deployment.

## Launch contract

`snapd` cannot launch the controller meaningfully without a valid
`runtime.conf`. The hard launch input is `agents/controller-0/runtime.conf`
with all minimum required fields filled in with valid values. The template
`runtime.conf.example` lists every field that `runtime.conf` validation
requires.

## Sensitive material

Do not hand-author TLS private key material. Source `ca-private-key`,
`controller-private-key`, and `agent-password` from bootstrap output or a
purpose-built test fixture. `runtime.conf` must be written with `0600`
permissions.

## Verification

This section records the reproducible bring-up procedure and the expected
results for the JUJU-10105 minimal runnable-service slice.

### Bring-up flow

1. Build the local snap artifact:
   ```bash
   make jujud-snap
   ```

2. Install it:
   ```bash
   sudo snap install --dangerous ./jujud_*.snap
   ```

3. Stage the local developer fixture under
   `/var/snap/jujud/common/var/lib/juju/`, with at least
   `agents/controller-0/runtime.conf` (from `runtime.conf.example`) and any
   extra controller data required for the chosen test fixture.

4. Start the service:
   ```bash
   sudo snap start jujud
   ```

5. Inspect the service status:
   ```bash
   snap services jujud
   ```

6. Inspect the service logs:
   ```bash
   sudo snap logs jujud -n 100
   ```

### Expected results

The expected result for this slice is **not** "the controller is fully ready
for bootstrap". The expected result is:

- the process does not exit immediately with command-usage or missing-flag
  errors;
- logs show that `runtime.conf` was consulted and controller startup began;
- any remaining failures are now attributable to deeper runtime, confinement,
  or bootstrap prerequisites rather than an invalid snapd service definition.

| Outcome | Disposition |
|---|---|
| Process exits with `--controller-id must be set` / `--data-dir option must be set` | Service command contract broken; step 1 regression |
| Process exits with `cannot read controller runtime config` | Fixture not staged or path contract broken; step 2/3 regression |
| Process starts, reads `runtime.conf`, then fails on deeper bootstrap/db/confinement prerequisites | Success for this slice; deeper failure recorded as follow-up |
| Process starts and reaches full controller readiness | Exceeds this slice; not required |


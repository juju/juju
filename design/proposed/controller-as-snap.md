# Controller-as-a-Snap: Implementation Roadmap

## Background

Juju's IAAS controller currently runs as a statically-linked binary
(`jujud-controller`) compiled with musl/CGO and distributed as a `tools.tar.gz`
archive. This document describes a roadmap to replace that with a proper Ubuntu
snap installed and upgraded on the controller machine, and to properly separate
the controller binary from the machine/unit agent binary.

## Problem statement

The current IAAS controller binary distribution has several interrelated problems:

**Musl toolchain and static build complexity.** The build requires musl-gcc and
pre-built static C libraries (`dqlite`, `raft`, `libuv`) downloaded into `_deps/`
from S3 at build time. This slows CI, complicates developer setup, and ties the
binary to a static-only distribution model.

**No binary separation.** Both `jujud` (machine/unit agent) and `jujud-controller`
(controller) link against the same packages, including dqlite and domain services.
Machine agents carry significant controller-only dependencies unnecessarily.

**Non-standard binary distribution.** Controllers receive binary updates through
a bespoke tools-tarball mechanism backed by simplestreams, a metadata discovery
system designed for cloud images. There is no standard rollback, hold, or
upgrade-gate mechanism.

**Simplestreams is a poor fit for controller binary distribution.** Simplestreams was
designed for cloud image metadata. Maintaining streams for controller binaries adds
operational overhead and prevents a clean upgrade path.

**HA transition is coupled to binary identity.** When a machine agent promotes to a
controller today, it restarts the same binary with a different command. With snap
and binary separation, it must install a different binary entirely, requiring
coordination with the snap install flow.

## Success criteria

The project is complete when:

- New IAAS controller installs use the `jujud-controller` snap by default; no
  `tools.tar.gz` is placed on the controller machine.
- Controllers can be upgraded by upgrading the snap revision; `snap refresh --hold`
  prevents automatic upgrades; the upgrader worker manages revision selection.
- HA clusters work: machine agents detect the controller role, install the snap, and
  transition cleanly to `jujud-controller`.
- `jujud` (machine/unit agent) and `jujud-controller` (controller snap) are truly
  separate binaries; `jujud` no longer links dqlite or domain services.
- The `jujud-controller` binary in the snap and in the CAAS OCI image are identical,
  verified by SHA256 hash in CI.
- Airgap deployments work via a snap store proxy or via pre-seeded snap+assert blobs
  in the dqlite object store.
- The musl-gcc toolchain and `_deps/` static C library downloads are removed from
  the build system.
- Simplestreams is no longer used for controller binary distribution (machine/unit
  agent simplestreams distribution is out of scope for this project).

## Decisions

**Separate snap repository.** Launchpad can only publish one snap per repository.
The `jujud-controller` snap is published from a new dedicated repository
`github.com/juju/jujud-controller-snap`; the `juju` CLI snap remains on Launchpad
from this repository.

**Snap and OCI must use the same binary.** The `jujud-controller` binary embedded
in the snap and in the CAAS OCI image must have an identical SHA256 hash. This is
achieved by both artifacts downloading the same pre-built binary from S3 during
their respective builds.

**Binary distribution via S3.** The main repository CI builds `jujud-controller`
and uploads it to S3. The snap repository and the OCI build both download and
package that same binary.

**`snap install --dangerous` is a dev-only tool.** It is acceptable in Stage 1 for
local developer workflow. The Stage 2 production path uses `snap download` →
`snap ack` → `snap install ./` from the snap store.

**Auto-refresh is held.** `snap refresh --hold jujud-controller` is run immediately
after install. The upgrader worker controls all snap upgrades explicitly.

**Assert files are stored in the object store.** The `.assert` file produced by
`snap download` is stored alongside the `.snap` blob in the dqlite object store,
ensuring both are available to all controller units via raft replication.

**Airgap deployments reuse existing snap proxy config.** The existing
`SnapStoreAssertions/ProxyID/URL` fields in `instancecfg.go` are reused. When
fully offline, snap+assert blobs are served from the dqlite object store.

**HA and binary separation are coupled.** The machine agent must install the
`jujud-controller` snap when transitioning to a controller role. This coupling
means binary separation and HA snap support are delivered together in Stage 4.

**Upgrades go through the object store.** The upgrader worker downloads the new
snap+assert from the dqlite object store and runs `snap ack` + `snap install ./`.

**CAAS is unaffected by the snap path.** CAAS controllers run in OCI containers
and do not use the snap install/upgrade flow.

**musl and `_deps` removed late.** The musl-gcc toolchain and the `_deps/`
pre-built static C libraries downloaded from S3 at build time are removed in
the final phase, after the snap path is fully the default. The S3 release artifacts
(built binaries: `jujud-controller`, `jujud`, `jujuc`, etc.) are **kept** —
they are used by `juju-release-jenkins` jobs.

**Binary separation is delayed.** Binary separation is deferred until Stage 4 to
avoid a short-lived period where two separate binaries must be maintained in
simplestreams before snap takes over.

## Affected Code Areas

This section describes the major code areas that will be modified during the
snap migration. Understanding these areas helps with planning and executing the
work.

**Snap Build Infrastructure:** The snap build system needs to move to a
separate repository because Launchpad can only publish one snap per repository.
The new `github.com/juju/jujud-controller-snap` repo will contain the snapcraft
definition, CI workflows for building and publishing to the snap store, and
logic to download the pre-built `jujud-controller` binary from S3 during snap
build. This separation allows the main repo to continue building the CLI snap
on Launchpad while the controller snap is built via GitHub Actions.

**Feature Flag System:** A new `ControllerSnap` feature flag gates all
snap-specific runtime behavior throughout the codebase. This allows the snap
path to be developed, tested, and refined while keeping the existing
tools.tar.gz path as the default. The flag is checked at key decision points:
bootstrap, upgrade, HA operations, and binary distribution. Once the snap path
is proven stable, the flag default flips to true, and eventually the flag is
removed entirely along with the legacy code paths.

**Cloud-Init and Bootstrap:** The cloud-init configuration generates shell
scripts that run on newly created controller machines to install and configure
the agent. Currently it downloads and extracts tools.tar.gz archives. With the
snap path, it needs to support three modes: snap download from the store
(production), snap install from local file (development), and snap install via
snap proxy (airgap). The bootstrap process also needs to store the snap and
assert files in the dqlite object store for HA replication.

**Agent Binary Storage Domain:** The agent binary domain manages metadata and
storage of agent binaries in the controller's dqlite database and object store.
This includes DDL schema, domain services, and repository interfaces. The
schema needs to be extended to track snap revisions, assert file paths, and
binary hashes. New service methods handle storing and retrieving snap+assert
blobs from the object store, which is replicated across all HA controller units
via dqlite raft.

**Bootstrap Worker:** The bootstrap worker orchestrates the controller
bootstrap sequence, including uploading the initial agent binary to the object
store. It needs a new code path to handle snap+assert files instead of
tools.tar.gz archives. The worker detects whether the snap flag is enabled and
dispatches to either the legacy PopulateAgentBinary function or the new
PopulateSnapAgentBinary function. This keeps both paths functional during the
transition period.

**Upgrader Worker:** The upgrader worker runs on every agent and watches for
new versions, downloads binaries, and triggers restarts. For controllers using
the snap path, it needs to download snap+assert files from the object store
(via API endpoints), run snap ack and snap install commands, and apply snap
refresh hold to prevent automatic upgrades. The upgrade coordination between
multiple HA controller units remains unchanged since the object store already
handles replication.

**API Binary Endpoints:** The controller API serves agent binaries to machines
via HTTP endpoints in apiserver/tools.go. New endpoints are needed to serve
snap and assert files separately from the object store. These endpoints
authenticate requests, read blobs from the object store, and stream them to
clients. The existing tools.tar.gz endpoints remain for machine and unit
agents, while controller agents use the new snap endpoints when the flag is
enabled.

**Simplestreams Binary Discovery:** Simplestreams is used to discover and
download agent binaries from public mirrors when the controller doesn't have
them in its object store. Within this project, the goal is to remove the
**controller** (`jujud-controller`) binary from simplestreams — controllers will
be distributed exclusively via snap. The machine/unit agent (`jujud`) binary
remains in simplestreams and is out of scope for this project. A future project
may address full simplestreams removal for agent binaries, but the replacement
distribution mechanism for machine/unit agents is an open question.

**Binary Separation:** Currently both `jujud` and `jujud-controller` link
against all packages including dqlite and domain services. True separation
means refactoring cmd/jujud to exclude controller-only imports, updating
Makefile build tags and linkage, and stopping the rename of jujud-controller to
jujud. Machine agents will run a smaller jujud binary without dqlite
dependencies. This separation is coupled with HA because the
machine-to-controller transition requires installing a different binary (the
snap).

**HA Snap Installation Logic:** When a machine agent determines it should
become a controller (via HA), it needs to install the jujud-controller snap
before restarting. This requires new detection logic in the machine agent
startup, an API endpoint to download snap+assert from the controller's object
store, local snap installation commands, and proper error handling for
installation failures. An alternative solution is to start the agent as a
controller from the beginning.

**Makefile and Build System:** The Makefile currently uses musl-gcc for static
linking and downloads pre-built dqlite/raft/libuv static libraries into the
`_deps/` folder from S3 at build time to speed up compilation. During the
transition, both the static binary (for legacy path) and dynamic binary (for
snap) need to build. Eventually the musl toolchain and the `_deps/` S3
downloads are removed, switching to apt-provided dynamic libraries. Note: the
S3 release artifacts (final built binaries: `jujud-controller`, `jujud`, `jujuc`,
etc.) are distinct from the build-time static libs — the release artifacts
remain in S3 and continue to be used by `juju-release-jenkins` jobs. The
Makefile also needs to stop renaming jujud-controller to jujud, allowing both
binaries to coexist with their proper names.

**CI/CD Binary Distribution:** The juju-release-jenkins jobs orchestrate
building, testing, and publishing releases. These jobs need to handle building
two separate binaries (jujud and jujud-controller), uploading both to S3 with
distinct names and paths, updating simplestreams metadata to include both, and
coordinating with the snap repository to ensure the snap build downloads the
correct controller binary. Hash verification checks ensure the snap and OCI
images use identical controller binaries.

**OCI Image Build:** The CAAS controller runs in Kubernetes as an
OCI container image, currently built via a Dockerfile that copies pre-built
binaries from the local build directory. The Dockerfile will be updated to
reference the renamed `jujud-controller` binary and download it from S3,
ensuring the OCI image and the snap use the exact same binary with identical
SHA256 hash. A future migration to Rockcraft (Canonical's OCI build tool) is
an open question.

## Implementation stages

### Stage 1 — Snap Infrastructure & Dev Workflow
**Outcome:** Developers can bootstrap and upgrade a single controller using a locally built snap file.

**Deliverables:**
- `github.com/juju/jujud-controller-snap` repo created with CI
- `ControllerSnap` feature flag added, default OFF
- `juju bootstrap --controller-snap=<path>` works end-to-end
- `juju upgrade-controller --controller-snap=<path>` works end-to-end
- Snap and assert stored in dqlite object store
- `jujud-controller` is built as it's now

**Problems to resolve:**
- Where snap metadata (revision, assert path) lives in the schema
- How to reject HA operations cleanly when flag is ON
- `snap install --dangerous` acceptable for dev; not for production

### Stage 2 — IAAS Production: Snap Store (Single Controller)
**Outcome:** Controllers bootstrap and upgrade from the snap store without local snap files.

**Deliverables:**
- Full `snap download` → `snap ack` → `snap install ./` flow in cloud-init
- Airgap bootstrap works via snap store proxy
- Simplestreams bypassed for controller binary discovery when flag is ON
- Connected upgrades pull new snap from snap store automatically
- `jujud-controller` is built as it's now

**Problems to resolve:**
- Snap channel strategy (`latest/stable` vs `4.x/stable`)
- How `upgrade-controller` discovers available snap revisions
- Fully offline airgap: no snap proxy, no internet access

### Stage 3 — Binary Separation + HA
**Outcome:** `jujud` and `jujud-controller` are truly separate binaries; HA works with the snap path.

**Deliverables:**
- `jujud` no longer links dqlite, raft, or domain services
- Machine agent detects controller role and installs snap automatically
- HA: all controller units run `jujud-controller` snap
- `juju add-unit controller` unblocked and functional with snap path

**Problems to resolve:**
- How machine agent detects it should become a controller
- Atomic and recoverable snap install during HA transition
- Simplestreams updates for two separate binaries (brief)
- Binary separation without breaking existing deployments

### Stage 4 — Binary Distribution Strategy + CAAS
**Outcome:** CAAS OCI image is updated to use the renamed `jujud-controller` binary; snap and OCI use the **same binary**.

**Deliverables:**
- `caas/Dockerfile` updated to reference `jujud-controller` binary (following Stage 3 rename)
- Both snap and OCI download the same binary from S3 during build
- CI enforces hash identity: snap binary SHA256 == OCI binary SHA256

**Problems to resolve:**
- Ensuring hash identity is verified before publishing
- S3 access credentials for snap repo and OCI builds

### Stage 5 — Flag Default ON & Full Integration Tests
**Outcome:** All new IAAS installs use the snap path by default.

**Deliverables:**
- `ControllerSnap` flag flipped to `true` by default
- Full integration test suite: bootstrap, upgrade, HA, airgap, binary separation
- Upgrade documentation for operators

**Problems to resolve:**
- Migration path for existing legacy-binary controllers
- All integration tests passing across all supported clouds

### Stage 6 — Legacy Removal
**Outcome:** All legacy binary-distribution code removed; snap is the only controller path.

**Deliverables:**
- `ControllerSnap` feature flag deleted entirely
- `tools.tar.gz` controller distribution code removed
- musl-gcc toolchain and `_deps/` S3 static library downloads removed
- `caas/Dockerfile` deleted or replaced (see Open Questions)
- Simplestreams removed for **controller** binary distribution; machine/unit agents unaffected
- Remove legacy build from jenkins release

**Problems to resolve:**
- Ensuring no controller code paths depend on removed infrastructure

---

## Open Questions

**How does the machine agent detect it should become a controller?** The detection
mechanism — whether via API query, local dqlite config, or a new signal — must be
decided.

**Upgrade procedure for snaps:** The method for upgrading production
controllers when using snaps is still to be determined.

**How are assert files handled in fully airgapped upgrades?** When there is no
snap proxy, the admin must supply snap and assert via
`juju upgrade-controller --controller-snap`. The exact workflow needs documenting.

**What happens if snap install fails during HA transition?** Retry logic, error
handling, and rollback strategy must be designed. A failed transition must not
leave the machine in an inconsistent state.

**Should `caas/Dockerfile` be replaced with Rockcraft?** Rockcraft is Canonical's
standard toolchain for OCI images. The Dockerfile is simpler to maintain
short-term but diverges from the direction of the wider platform team.

**How to eliminate simplestreams for machine/unit agent binaries?** This is out of
scope for this project but needs a dedicated decision. Options include bundling
agent binaries in the controller snap, a separate snap, or charm payload.

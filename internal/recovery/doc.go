// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package recovery implements the recovery machinery composing the
// recovery domain (domain/recovery).
//
// The offline half runs on the bootstrap client before anything is
// provisioned: archive reading and validation (ValidateArchive,
// ReadArchive), the SHA-256 checksum verification, and the dump decoding
// that produces the archive summary driving the recovery preflight
// checks. The archive is operator-supplied but never trusted beyond its
// checksum. When the archive carries a content manifest (manifest.json),
// reading cross-checks it against the actual entries in both directions:
// the manifest is an index of the archive, never a source of truth.
//
// Model dumps are staged in temporary files under the system's
// temporary directory (TMPDIR), each bounded by MaxDumpSize: recovering
// an archive needs enough free space there to hold the archived model
// dumps.
//
// The agent-side half is the recovery bootstrap orchestration: the stage
// reads and validates the uploaded archive, loads the archived dumps
// into the freshly bootstrapped databases, installs the object blobs
// into the replacement's file-backed object store, patches the
// replacement's physical facts in place and pins the file-backed object
// store until the operator authorizes a transition. The stage runs
// inside dqlite bootstrap, after the schema migrations and before any
// agent, API or worker starts, with every database exclusively owned by
// bootstrap.
package recovery

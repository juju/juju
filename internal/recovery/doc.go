// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package recovery implements the offline half of controller recovery:
// archive reading and validation (ValidateArchive, ReadArchive), the
// SHA-256 checksum verification, and the dump decoding that produces the
// archive summary (domain/recovery ArchiveInfo) driving the recovery
// preflight checks. It runs on the bootstrap client before anything is
// provisioned: validation is offline and read-only, and the archive is
// operator-supplied but never trusted beyond its checksum. When the
// archive carries a content manifest (manifest.json), reading
// cross-checks it against the actual entries in both directions: the
// manifest is an index of the archive, never a source of truth.
package recovery

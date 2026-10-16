// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package recovery defines the domain vocabulary for controller recovery
// from a backup archive: the validated archive summary (ArchiveInfo,
// ModelInfo, ApplicationInfo) and the preflight gates over it
// (CheckAgentVersion, CheckProviderFamily). The replacement
// controller is bootstrapped with the source's identity and its
// databases loaded from the archived dumps; the summary and gates are
// what the bootstrap client decides from.
//
// The offline archive handling — reading the tarball, verifying its
// checksum, and decoding the dumps into the summary — lives in
// internal/recovery, which composes this domain.
package recovery

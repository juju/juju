// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

//+build !windows

// Package transientfile provides helpers for creating files that do not
// survive machine reboots.
package transientfile

// ensureDeleteAfterReboot is not required for *nix targets as Create expects
// that the caller provides a true transient folder (e.g. a tmpfs mount).
func ensureDeleteAfterReboot(string) error { return nil }

// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SnapConfinementHint returns a hint message when running as a snap and path
// is outside the snap's reachable directories (HOME / SNAP_REAL_HOME).
// Returns "" if not running as a snap, if the path is under a reachable root,
// or if path does not look like a filesystem path (no '/' separator).
//
// snapEnv is $SNAP, snapRealHome is $SNAP_REAL_HOME, homeDir is $HOME.
func SnapConfinementHint(path, snapEnv, snapRealHome, homeDir string) string {
	if snapEnv == "" {
		return ""
	}
	// Only trigger for path-like arguments (contains a directory separator).
	if !strings.Contains(path, "/") {
		return ""
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	if isUnderRoot(abs, homeDir) || isUnderRoot(abs, snapRealHome) {
		return ""
	}
	return fmt.Sprintf(
		"\n\nThe Juju snap is strictly confined and cannot access files outside your home\n"+
			"directory. Move the file into your home directory and try again, for example:\n\n"+
			"    cp %s ~/", path)
}

// SnapConfinementHintFromEnv calls SnapConfinementHint using the process environment.
func SnapConfinementHintFromEnv(path string) string {
	return SnapConfinementHint(path, os.Getenv("SNAP"), os.Getenv("SNAP_REAL_HOME"), os.Getenv("HOME"))
}

// isUnderRoot reports whether path is under root (root followed by a path separator).
func isUnderRoot(path, root string) bool {
	if root == "" {
		return false
	}
	return strings.HasPrefix(path, root+string(filepath.Separator))
}

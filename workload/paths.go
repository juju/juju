// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package workload

import (
	"path/filepath"
)

// DataDir returns the path to the top-level data directory for
// workloads relative to the provided base directory.
func DataDir(baseDataDir string) string {
	return filepath.Join(baseDataDir, ComponentName)
}

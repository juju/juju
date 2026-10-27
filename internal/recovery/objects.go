// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package recovery

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	domainrecovery "github.com/juju/juju/domain/recovery"
	"github.com/juju/juju/internal/errors"
)

// CheckObjectStore verifies that every object metadata row recorded in
// the recovered databases resolves to a blob in the replacement's object
// store. Object metadata lives in the controller and model databases and
// a complete local object set is a hard requirement for recovery: a
// merged metadata row without its blob would only fail later, on first
// Get, so an incomplete set fails the load here. A blob missing from the
// store was not in the archive — or was a special entry the unpack
// refused to write.
func CheckObjectStore(ctx context.Context, inventories []domainrecovery.ObjectInventory, dataDir string) error {
	var missing []string
	for _, inventory := range inventories {
		if err := ctx.Err(); err != nil {
			return errors.Capture(err)
		}
		for _, p := range inventory.Paths {
			blob := filepath.Join(dataDir, objectStoreDirName, inventory.Namespace, p.SHA384)
			if _, err := os.Stat(blob); errors.Is(err, os.ErrNotExist) {
				// A metadata row without a path is still
				// reachable by hash; report the hash.
				name := p.Path
				if name == "" {
					name = p.SHA384
				}
				missing = append(missing, inventory.Namespace+"/"+name)
			} else if err != nil {
				return errors.Capture(err)
			}
		}
	}
	if len(missing) > 0 {
		return errors.Errorf(
			"recovered object metadata references blobs that are not in the archive: %s",
			strings.Join(missing, "; "))
	}
	return nil
}

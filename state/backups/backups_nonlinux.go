// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !linux

package backups

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"
)

// Restore satisfies the Backups interface on non-Linux OSes (e.g.
// windows, darwin).
func (*backups) Restore(_ string, _ RestoreArgs) (names.Tag, error) {
	return nil, errors.Errorf("backups supported only on Linux")
}

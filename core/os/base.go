// Copyright 2024 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package os

import (
	"sync"

	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/internal/errors"
)

var (
	// HostBase returns the base of the machine the current process is
	// running on (overrideable var for testing)
	HostBase func() (corebase.Base, error) = hostBase

	baseOnce sync.Once

	// These are filled in by the first call to hostBase
	base    corebase.Base
	baseErr error
)

func hostBase() (corebase.Base, error) {
	var err error
	baseOnce.Do(func() {
		base, err = readBase()
		if err != nil {
			baseErr = errors.Errorf("cannot determine host base: %w", err)
		}
	})
	return base, baseErr
}

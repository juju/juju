// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !go1.3

package lxd

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/container"
)

var (
	logger = loggo.GetLogger("juju.container.lxd")
)

const (
	DefaultLxdBridge = "INVALIDBRIDGEGO1.2"
)

func NewContainerManager(conf container.ManagerConfig) (container.Manager, error) {
	return nil, errors.Errorf("LXD containers not supported in go 1.2")
}

func NewContainerInitialiser(series string) container.Initialiser {
	logger.Errorf("No LXD container initializer in go 1.2")
	/* while it seems slightly impolite to return nil here, the return
	 * value is never actually used, because it's never deref'd before
	 * NewContainerManager is called, which *does* actually return an
	 * error that bubbles up.
	 */
	return nil
}

func HasLXDSupport() bool {
	return false
}

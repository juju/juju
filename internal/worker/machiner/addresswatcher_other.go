// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

//go:build !linux

package machiner

import (
	"context"

	"github.com/juju/juju/core/watcher"
)

func newAddressChangeNotifyWatcher(context.Context) (watcher.NotifyWatcher, error) {
	return nil, nil
}

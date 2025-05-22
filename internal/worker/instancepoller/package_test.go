// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancepoller

import (
	stdtesting "testing"

	"github.com/juju/tc"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/mocks_watcher.go github.com/juju/juju/core/watcher StringsWatcher
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/mocks_instances.go github.com/juju/juju/environs/instances Instance
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/mocks_instancepoller.go github.com/juju/juju/internal/worker/instancepoller Environ,Machine


// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package keyupdater

import (
	stdtesting "testing"

	"github.com/juju/tc"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package keyupdater -destination service_mock_test.go github.com/juju/juju/apiserver/facades/agent/keyupdater KeyUpdaterService
//go:generate go run go.uber.org/mock/mockgen -typed -package keyupdater -destination facade_mock_test.go github.com/juju/juju/apiserver/facade ModelContext


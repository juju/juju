// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialvalidator

import (
	stdtesting "testing"

	"github.com/juju/tc"
)

//go:generate go run go.uber.org/mock/mockgen  -typed -package credentialvalidator -destination services_mock.go github.com/juju/juju/apiserver/facades/agent/credentialvalidator ModelCredentialService
//go:generate go run go.uber.org/mock/mockgen  -typed -package credentialvalidator -destination watcher_mock.go github.com/juju/juju/core/watcher NotifyWatcher


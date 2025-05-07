// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshclient

import (
	"testing"

	"github.com/juju/tc"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package sshclient_test -destination leadership_mock_test.go github.com/juju/juju/core/leadership Reader
//go:generate go run go.uber.org/mock/mockgen -typed -package sshclient_test -destination state_mock_test.go github.com/juju/juju/apiserver/facades/client/sshclient Backend,SSHMachine
//go:generate go run go.uber.org/mock/mockgen -typed -package sshclient_test -destination authorizer_mock_test.go github.com/juju/juju/apiserver/facade Authorizer
//go:generate go run go.uber.org/mock/mockgen -typed -package sshclient_test -destination service_mock_test.go github.com/juju/juju/apiserver/facades/client/sshclient ModelConfigService,ModelProviderService

func Test(t *testing.T) {
	tc.TestingT(t)
}

// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshclient

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package sshclient_test -destination leadership_mock_test.go github.com/juju/juju/core/leadership Reader
//go:generate go run go.uber.org/mock/mockgen -package sshclient_test -destination state_mock_test.go github.com/juju/juju/apiserver/facades/client/sshclient Backend,Model,Broker,SSHMachine
//go:generate go run go.uber.org/mock/mockgen -package sshclient_test -destination authorizer_mock_test.go github.com/juju/juju/apiserver/facade Authorizer
//go:generate go run go.uber.org/mock/mockgen -package sshclient_test -destination service_mock_test.go github.com/juju/juju/apiserver/facades/client/sshclient ModelConfigService

func Test(t *testing.T) {
	gc.TestingT(t)
}

// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack_test

import (
	stdtesting "testing"

	"github.com/juju/tc"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package openstack -destination network_mock_test.go github.com/juju/juju/internal/provider/openstack SSLHostnameConfig,Networking,NetworkingBase,NetworkingNeutron,NetworkingAuthenticatingClient,NetworkingNova,NetworkingEnvironConfig
//go:generate go run go.uber.org/mock/mockgen -typed -package openstack -destination cloud_mock_test.go github.com/juju/juju/internal/cloudconfig/cloudinit NetworkingConfig
//go:generate go run go.uber.org/mock/mockgen -typed -package openstack -destination goose_mock_test.go github.com/go-goose/goose/v5/client AuthenticatingClient

func Test(t *stdtesting.T) {
	tc.TestingT(t)
}

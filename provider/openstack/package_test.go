// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack_test

import (
	"flag"
	"testing"

	"github.com/go-goose/goose/v5/identity"
	gc "gopkg.in/check.v1"
)

//go:generate go run github.com/golang/mock/mockgen -package openstack -destination network_mock_test.go github.com/juju/juju/provider/openstack SSLHostnameConfig,Networking,NetworkingBase,NetworkingNeutron,NetworkingAuthenticatingClient,NetworkingNova,NetworkingEnvironConfig
//go:generate go run github.com/golang/mock/mockgen -package openstack -destination cloud_mock_test.go github.com/juju/juju/cloudconfig/cloudinit NetworkingConfig
//go:generate go run github.com/golang/mock/mockgen -package openstack -destination goose_mock_test.go github.com/go-goose/goose/v5/client AuthenticatingClient

var live = flag.Bool("live", false, "Include live OpenStack tests")

func Test(t *testing.T) {
	if *live {
		cred, err := identity.CompleteCredentialsFromEnv()
		if err != nil {
			t.Fatalf("Error setting up test suite: %s", err.Error())
		}
		registerLiveTests(cred)
	}
	registerLocalTests()
	gc.TestingT(t)
}

// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure_test

import (
	"net/http"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/provider/azure"
	"github.com/juju/juju/provider/azure/internal/azuretesting"
)

type AuthSuite struct {
	testing.IsolationSuite
	requests []*http.Request
}

var _ = gc.Suite(&AuthSuite{})

func (s *AuthSuite) TestAuthTokenServicePrincipalSecret(c *gc.C) {
	spec := environs.CloudSpec{
		Type:             "azure",
		Name:             "azure",
		Region:           "westus",
		Endpoint:         "https://api.azurestack.local",
		IdentityEndpoint: "https://graph.azurestack.local",
		StorageEndpoint:  "https://storage.azurestack.local",
		Credential:       fakeServicePrincipalCredential(),
	}
	senders := azuretesting.Senders{
		discoverAuthSender(),
	}
	token, err := azure.AuthToken(spec, &senders)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(token, gc.NotNil)
}

func (s *AuthSuite) TestAuthTokenInteractive(c *gc.C) {
	spec := environs.CloudSpec{
		Type:             "azure",
		Name:             "azure",
		Region:           "westus",
		Endpoint:         "https://api.azurestack.local",
		IdentityEndpoint: "https://graph.azurestack.local",
		StorageEndpoint:  "https://storage.azurestack.local",
		Credential:       fakeInteractiveCredential(),
	}
	senders := azuretesting.Senders{}
	_, err := azure.AuthToken(spec, &senders)
	c.Assert(err, gc.ErrorMatches, `auth-type "interactive" not supported`)
}

func fakeInteractiveCredential() *cloud.Credential {
	cred := cloud.NewCredential("interactive", map[string]string{
		"subscription-id": fakeSubscriptionId,
	})
	return &cred
}

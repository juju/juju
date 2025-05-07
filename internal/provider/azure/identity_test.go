// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/environs"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/internal/testing"
)

const (
	fakeSubscriptionId = "22222222-2222-2222-2222-222222222222"
)

type identitySuite struct {
	testing.BaseSuite
}

var _ = tc.Suite(&identitySuite{})

func (s *identitySuite) TestFinaliseBootstrapCredentialNoInstanceRole(c *tc.C) {
	env := azureEnviron{subscriptionId: fakeSubscriptionId}
	cred := cloud.NewCredential("service-principal-secret", map[string]string{
		"application-id":          "application",
		"application-password":    "password",
		"subscription-id":         "subscription",
		"managed-subscription-id": "managed-subscription",
	})
	ctx := envtesting.BootstrapTestContext(c)
	args := environs.BootstrapParams{}
	got, err := env.FinaliseBootstrapCredential(ctx, args, &cred)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, jc.DeepEquals, &cred)
}

func (s *identitySuite) TestFinaliseBootstrapCredentialInstanceRole(c *tc.C) {
	env := azureEnviron{subscriptionId: fakeSubscriptionId}
	cred := cloud.NewCredential("service-principal-secret", map[string]string{
		"application-id":          "application",
		"application-password":    "password",
		"subscription-id":         "subscription",
		"managed-subscription-id": "managed-subscription",
	})
	ctx := envtesting.BootstrapTestContext(c)
	args := environs.BootstrapParams{
		BootstrapConstraints: constraints.MustParse("instance-role=foo"),
	}
	got, err := env.FinaliseBootstrapCredential(ctx, args, &cred)
	c.Assert(err, jc.ErrorIsNil)
	want := cloud.NewCredential("managed-identity", map[string]string{
		"subscription-id":       fakeSubscriptionId,
		"managed-identity-path": "foo",
	})
	c.Assert(got, jc.DeepEquals, &want)
}

func (s *identitySuite) TestManagedIdentityGroup(c *tc.C) {
	env := azureEnviron{resourceGroup: "some-group"}
	c.Assert(env.managedIdentityGroup("myidentity"), tc.Equals, "some-group")
	c.Assert(env.managedIdentityGroup("mygroup/myidentity"), tc.Equals, "mygroup")
	c.Assert(env.managedIdentityGroup("mysubscription/mygroup/myidentity"), tc.Equals, "mygroup")
}

func (s *identitySuite) TestManagedIdentityResourceId(c *tc.C) {
	env := azureEnviron{resourceGroup: "some-group", subscriptionId: fakeSubscriptionId}
	c.Assert(env.managedIdentityResourceId("myidentity"), tc.Equals, "/subscriptions/22222222-2222-2222-2222-222222222222/resourcegroups/some-group/providers/Microsoft.ManagedIdentity/userAssignedIdentities/myidentity")
	c.Assert(env.managedIdentityResourceId("mygroup/myidentity"), tc.Equals, "/subscriptions/22222222-2222-2222-2222-222222222222/resourcegroups/mygroup/providers/Microsoft.ManagedIdentity/userAssignedIdentities/myidentity")
	c.Assert(env.managedIdentityResourceId("mysubscription/mygroup/myidentity"), tc.Equals, "/subscriptions/mysubscription/resourcegroups/mygroup/providers/Microsoft.ManagedIdentity/userAssignedIdentities/myidentity")
}

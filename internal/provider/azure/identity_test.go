// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/environs"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/testing"
)

const (
	fakeSubscriptionId = "22222222-2222-2222-2222-222222222222"
)

type identitySuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&identitySuite{})

func (s *identitySuite) TestFinaliseBootstrapCredentialNoInstanceRole(c *gc.C) {
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

func (s *identitySuite) TestFinaliseBootstrapCredentialInstanceRole(c *gc.C) {
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
	want := cloud.NewCredential("instance-role", map[string]string{
		"subscription-id":  fakeSubscriptionId,
		"managed-identity": "foo",
	})
	c.Assert(got, jc.DeepEquals, &want)
}

func (s *identitySuite) TestManagedIdentityGroup(c *gc.C) {
	env := azureEnviron{resourceGroup: "some-group"}
	c.Assert(env.managedIdentityGroup("myidentity"), gc.Equals, "some-group")
	c.Assert(env.managedIdentityGroup("mygroup/myidentity"), gc.Equals, "mygroup")
	c.Assert(env.managedIdentityGroup("mysubscription/mygroup/myidentity"), gc.Equals, "mygroup")
}

func (s *identitySuite) TestManagedIdentityResourceId(c *gc.C) {
	env := azureEnviron{resourceGroup: "some-group", subscriptionId: fakeSubscriptionId}
	c.Assert(env.managedIdentityResourceId("myidentity"), gc.Equals, "/subscriptions/22222222-2222-2222-2222-222222222222/resourcegroups/some-group/providers/Microsoft.ManagedIdentity/userAssignedIdentities/myidentity")
	c.Assert(env.managedIdentityResourceId("mygroup/myidentity"), gc.Equals, "/subscriptions/22222222-2222-2222-2222-222222222222/resourcegroups/mygroup/providers/Microsoft.ManagedIdentity/userAssignedIdentities/myidentity")
	c.Assert(env.managedIdentityResourceId("mysubscription/mygroup/myidentity"), gc.Equals, "/subscriptions/mysubscription/resourcegroups/mygroup/providers/Microsoft.ManagedIdentity/userAssignedIdentities/myidentity")
}

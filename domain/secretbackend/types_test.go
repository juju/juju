// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackend

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/secrets/provider"
	"github.com/juju/juju/internal/secrets/provider/kubernetes"
)

type typesSuite struct{}

func TestTypesSuite(t *testing.T) {
	tc.Run(t, &typesSuite{})
}

func (s *typesSuite) TestMakeBuiltInK8sSecretBackendName(c *tc.C) {
	c.Check(MakeBuiltInK8sSecretBackendName("foo"), tc.Equals, "foo-local")
}

func (s *typesSuite) TestIsBuiltInK8sSecretBackendName(c *tc.C) {
	c.Check(IsBuiltInK8sSecretBackendName(provider.Internal), tc.IsTrue)
	c.Check(IsBuiltInK8sSecretBackendName(kubernetes.BackendName), tc.IsTrue)
	c.Check(IsBuiltInK8sSecretBackendName("foo-local"), tc.IsTrue)
	c.Check(IsBuiltInK8sSecretBackendName("foo"), tc.IsFalse)
}

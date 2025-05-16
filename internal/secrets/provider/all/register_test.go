// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package all_test

import (
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/secrets/provider"
	_ "github.com/juju/juju/internal/secrets/provider/all"
	"github.com/juju/juju/internal/secrets/provider/juju"
	"github.com/juju/juju/internal/secrets/provider/kubernetes"
	"github.com/juju/juju/internal/secrets/provider/vault"
	"github.com/juju/juju/internal/testhelpers"
)

type allSuite struct {
	testhelpers.IsolationSuite
}

func TestAllSuite(t *stdtesting.T) { tc.Run(t, &allSuite{}) }
func (s *allSuite) TestInit(c *tc.C) {
	for _, name := range []string{
		juju.BackendType,
		kubernetes.BackendType,
		vault.BackendType,
	} {
		p, err := provider.Provider(name)
		c.Check(err, tc.ErrorIsNil)
		c.Check(p.Type(), tc.Equals, name)
	}
}

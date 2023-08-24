// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package all_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/secrets/provider"
	_ "github.com/juju/juju/internal/secrets/provider/all"
	"github.com/juju/juju/internal/secrets/provider/juju"
	"github.com/juju/juju/internal/secrets/provider/kubernetes"
	"github.com/juju/juju/internal/secrets/provider/vault"
)

type allSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&allSuite{})

func (s *allSuite) TestInit(c *gc.C) {
	for _, name := range []string{
		juju.BackendType,
		kubernetes.BackendType,
		vault.BackendType,
	} {
		p, err := provider.Provider(name)
		c.Check(err, jc.ErrorIsNil)
		c.Check(p.Type(), gc.Equals, name)
	}
}

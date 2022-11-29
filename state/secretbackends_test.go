// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"sort"
	"time"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/testing"
)

type SecretBackendsSuite struct {
	testing.StateSuite
	storage state.SecretBackendsStorage
}

var _ = gc.Suite(&SecretBackendsSuite{})

func (s *SecretBackendsSuite) SetUpTest(c *gc.C) {
	s.StateSuite.SetUpTest(c)
	s.storage = state.NewSecretBackends(s.State)
}

func (s *SecretBackendsSuite) TestCreate(c *gc.C) {
	config := map[string]interface{}{"foo.key": "bar"}
	p := state.CreateSecretBackendParams{
		Name:                "myvault",
		Backend:             "vault",
		TokenRotateInterval: ptr(666 * time.Minute),
		Config:              config,
	}
	err := s.storage.CreateSecretBackend(p)
	c.Assert(err, jc.ErrorIsNil)
	backend, err := s.storage.GetSecretBackend("myvault")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(backend, jc.DeepEquals, &secrets.SecretBackend{
		Name:                "myvault",
		Backend:             "vault",
		TokenRotateInterval: ptr(666 * time.Minute),
		Config:              config,
	})

	err = s.storage.CreateSecretBackend(p)
	c.Assert(err, jc.Satisfies, errors.IsAlreadyExists)
}

func (s *SecretBackendsSuite) TestGetNotFound(c *gc.C) {
	_, err := s.storage.GetSecretBackend("myvault")
	c.Check(err, jc.Satisfies, errors.IsNotFound)
}

func (s *SecretBackendsSuite) TestList(c *gc.C) {
	config := map[string]interface{}{"foo.key": "bar"}
	p := state.CreateSecretBackendParams{
		Name:                "myvault",
		Backend:             "vault",
		TokenRotateInterval: ptr(666 * time.Minute),
		Config:              config,
	}
	err := s.storage.CreateSecretBackend(p)
	c.Assert(err, jc.ErrorIsNil)
	p2 := state.CreateSecretBackendParams{
		Name:    "myk8s",
		Backend: "kubernetes",
		Config:  config,
	}
	err = s.storage.CreateSecretBackend(p2)
	c.Assert(err, jc.ErrorIsNil)
	backends, err := s.storage.ListSecretBackends()
	c.Assert(err, jc.ErrorIsNil)
	sort.Slice(backends, func(i, j int) bool {
		return backends[i].Name < backends[j].Name
	})

	c.Assert(backends, jc.DeepEquals, []*secrets.SecretBackend{{
		Name:    "myk8s",
		Backend: "kubernetes",
		Config:  config,
	}, {
		Name:                "myvault",
		Backend:             "vault",
		TokenRotateInterval: ptr(666 * time.Minute),
		Config:              config,
	}})
}

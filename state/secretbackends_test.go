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
		BackendType:         "vault",
		TokenRotateInterval: ptr(666 * time.Minute),
		Config:              config,
	}
	err := s.storage.CreateSecretBackend(p)
	c.Assert(err, jc.ErrorIsNil)
	backend, err := s.storage.GetSecretBackend("myvault")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(backend.ID, gc.NotNil)
	backend.ID = ""
	c.Assert(backend, jc.DeepEquals, &secrets.SecretBackend{
		Name:                "myvault",
		BackendType:         "vault",
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
		BackendType:         "vault",
		TokenRotateInterval: ptr(666 * time.Minute),
		Config:              config,
	}
	err := s.storage.CreateSecretBackend(p)
	c.Assert(err, jc.ErrorIsNil)
	p2 := state.CreateSecretBackendParams{
		Name:        "myk8s",
		BackendType: "kubernetes",
		Config:      config,
	}
	err = s.storage.CreateSecretBackend(p2)
	c.Assert(err, jc.ErrorIsNil)
	backends, err := s.storage.ListSecretBackends()
	c.Assert(err, jc.ErrorIsNil)
	sort.Slice(backends, func(i, j int) bool {
		return backends[i].Name < backends[j].Name
	})

	mc := jc.NewMultiChecker()
	mc.AddExpr(`_.ID`, gc.NotNil)
	c.Assert(backends, mc, []*secrets.SecretBackend{{
		Name:        "myk8s",
		BackendType: "kubernetes",
		Config:      config,
	}, {
		Name:                "myvault",
		BackendType:         "vault",
		TokenRotateInterval: ptr(666 * time.Minute),
		Config:              config,
	}})
}

func (s *SecretBackendsSuite) TestRemove(c *gc.C) {
	p := state.CreateSecretBackendParams{
		Name:        "myvault",
		BackendType: "vault",
	}
	err := s.storage.CreateSecretBackend(p)
	c.Assert(err, jc.ErrorIsNil)

	err = s.storage.DeleteSecretBackend("myvault", false)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.storage.GetSecretBackend("myvault")
	c.Check(err, jc.Satisfies, errors.IsNotFound)
	err = s.storage.DeleteSecretBackend("myvault", false)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *SecretBackendsSuite) TestRemoveWithRevisionsFails(c *gc.C) {
	p := state.CreateSecretBackendParams{
		Name:        "myvault",
		BackendType: "vault",
	}
	err := s.storage.CreateSecretBackend(p)
	c.Assert(err, jc.ErrorIsNil)
	b, err := s.storage.GetSecretBackend("myvault")
	c.Assert(err, jc.ErrorIsNil)

	owner := s.Factory.MakeApplication(c, nil)
	uri := secrets.NewURI()
	sp := state.CreateSecretParams{
		Version: 1,
		Owner:   owner.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &fakeToken{},
			ValueRef: &secrets.ValueRef{
				BackendID:  b.ID,
				RevisionID: "rev-id",
			},
		},
	}
	secrets := state.NewSecrets(s.State)
	_, err = secrets.CreateSecret(uri, sp)
	c.Assert(err, jc.ErrorIsNil)

	err = s.storage.DeleteSecretBackend("myvault", false)
	c.Assert(err, jc.Satisfies, errors.IsNotSupported)
	count, err := state.SecretBackendRefCount(s.State, b.ID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(count, gc.Equals, 1)
}

func (s *SecretBackendsSuite) TestRemoveWithRevisionsForce(c *gc.C) {
	p := state.CreateSecretBackendParams{
		Name:        "myvault",
		BackendType: "vault",
	}
	err := s.storage.CreateSecretBackend(p)
	c.Assert(err, jc.ErrorIsNil)
	b, err := s.storage.GetSecretBackend("myvault")
	c.Assert(err, jc.ErrorIsNil)

	owner := s.Factory.MakeApplication(c, nil)
	uri := secrets.NewURI()
	sp := state.CreateSecretParams{
		Version: 1,
		Owner:   owner.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &fakeToken{},
			ValueRef: &secrets.ValueRef{
				BackendID:  b.ID,
				RevisionID: "rev-id",
			},
		},
	}
	secrets := state.NewSecrets(s.State)
	_, err = secrets.CreateSecret(uri, sp)
	c.Assert(err, jc.ErrorIsNil)

	count, err := state.SecretBackendRefCount(s.State, b.ID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(count, gc.Equals, 1)

	err = s.storage.DeleteSecretBackend("myvault", true)
	c.Assert(err, jc.ErrorIsNil)
	_, err = state.SecretBackendRefCount(s.State, b.ID)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	_, err = s.storage.GetSecretBackend("myvault")
	c.Check(err, jc.Satisfies, errors.IsNotFound)
}

func (s *SecretBackendsSuite) TestDeleteSecretUpdatesRefCount(c *gc.C) {
	p := state.CreateSecretBackendParams{
		Name:        "myvault",
		BackendType: "vault",
	}
	err := s.storage.CreateSecretBackend(p)
	c.Assert(err, jc.ErrorIsNil)
	b, err := s.storage.GetSecretBackend("myvault")
	c.Assert(err, jc.ErrorIsNil)

	owner := s.Factory.MakeApplication(c, nil)
	uri := secrets.NewURI()
	cp := state.CreateSecretParams{
		Version: 1,
		Owner:   owner.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &fakeToken{},
			ValueRef: &secrets.ValueRef{
				BackendID:  b.ID,
				RevisionID: "rev-id",
			},
		},
	}
	secretStore := state.NewSecrets(s.State)
	_, err = secretStore.CreateSecret(uri, cp)
	c.Assert(err, jc.ErrorIsNil)
	_, err = secretStore.UpdateSecret(uri, state.UpdateSecretParams{
		LeaderToken: &fakeToken{},
		ValueRef: &secrets.ValueRef{
			BackendID:  b.ID,
			RevisionID: "rev-id2",
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	count, err := state.SecretBackendRefCount(s.State, b.ID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(count, gc.Equals, 2)

	_, err = secretStore.DeleteSecret(uri)
	c.Assert(err, jc.ErrorIsNil)

	count, err = state.SecretBackendRefCount(s.State, b.ID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(count, gc.Equals, 0)

	err = s.storage.DeleteSecretBackend("myvault", false)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *SecretBackendsSuite) TestDeleteRevisionsUpdatesRefCount(c *gc.C) {
	p := state.CreateSecretBackendParams{
		Name:        "myvault",
		BackendType: "vault",
	}
	err := s.storage.CreateSecretBackend(p)
	c.Assert(err, jc.ErrorIsNil)
	b, err := s.storage.GetSecretBackend("myvault")
	c.Assert(err, jc.ErrorIsNil)

	owner := s.Factory.MakeApplication(c, nil)
	uri := secrets.NewURI()
	cp := state.CreateSecretParams{
		Version: 1,
		Owner:   owner.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &fakeToken{},
			ValueRef: &secrets.ValueRef{
				BackendID:  b.ID,
				RevisionID: "rev-id",
			},
		},
	}
	secretStore := state.NewSecrets(s.State)
	_, err = secretStore.CreateSecret(uri, cp)
	c.Assert(err, jc.ErrorIsNil)
	_, err = secretStore.UpdateSecret(uri, state.UpdateSecretParams{
		LeaderToken: &fakeToken{},
		ValueRef: &secrets.ValueRef{
			BackendID:  b.ID,
			RevisionID: "rev-id2",
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	count, err := state.SecretBackendRefCount(s.State, b.ID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(count, gc.Equals, 2)

	_, err = secretStore.DeleteSecret(uri, 1)
	c.Assert(err, jc.ErrorIsNil)

	count, err = state.SecretBackendRefCount(s.State, b.ID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(count, gc.Equals, 1)

	_, err = secretStore.DeleteSecret(uri, 2)
	c.Assert(err, jc.ErrorIsNil)

	count, err = state.SecretBackendRefCount(s.State, b.ID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(count, gc.Equals, 0)

	err = s.storage.DeleteSecretBackend("myvault", false)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *SecretBackendsSuite) TestUpdate(c *gc.C) {
	p := state.CreateSecretBackendParams{
		Name:        "myvault",
		BackendType: "vault",
		Config:      map[string]interface{}{"foo.key": "bar"},
	}
	err := s.storage.CreateSecretBackend(p)
	c.Assert(err, jc.ErrorIsNil)
	b, err := s.storage.GetSecretBackend("myvault")
	c.Assert(err, jc.ErrorIsNil)

	u := state.UpdateSecretBackendParams{
		ID:                  b.ID,
		TokenRotateInterval: ptr(666 * time.Second),
		Config:              map[string]interface{}{"foo": "bar2"},
	}
	err = s.storage.UpdateSecretBackend(u)
	c.Assert(err, jc.ErrorIsNil)
	b, err = s.storage.GetSecretBackend("myvault")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(b, jc.DeepEquals, &secrets.SecretBackend{
		ID:                  b.ID,
		Name:                "myvault",
		BackendType:         "vault",
		TokenRotateInterval: ptr(666 * time.Second),
		Config:              map[string]interface{}{"foo": "bar2"},
	})
}

func (s *SecretBackendsSuite) TestUpdateName(c *gc.C) {
	p := state.CreateSecretBackendParams{
		Name:        "myvault",
		BackendType: "vault",
		Config:      map[string]interface{}{"foo.key": "bar"},
	}
	err := s.storage.CreateSecretBackend(p)
	c.Assert(err, jc.ErrorIsNil)
	b, err := s.storage.GetSecretBackend("myvault")
	c.Assert(err, jc.ErrorIsNil)

	u := state.UpdateSecretBackendParams{
		ID:         b.ID,
		NameChange: ptr("myvault2"),
		Config:     map[string]interface{}{"foo": "bar2"},
	}
	err = s.storage.UpdateSecretBackend(u)
	c.Assert(err, jc.ErrorIsNil)
	b, err = s.storage.GetSecretBackend("myvault2")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(b, jc.DeepEquals, &secrets.SecretBackend{
		ID:          b.ID,
		Name:        "myvault2",
		BackendType: "vault",
		Config:      map[string]interface{}{"foo": "bar2"},
	})
}

func (s *SecretBackendsSuite) TestUpdateNameDuplicate(c *gc.C) {
	p := state.CreateSecretBackendParams{
		Name:        "myvault",
		BackendType: "vault",
		Config:      map[string]interface{}{"foo.key": "bar"},
	}
	err := s.storage.CreateSecretBackend(p)
	c.Assert(err, jc.ErrorIsNil)
	p.Name = "myvault2"
	err = s.storage.CreateSecretBackend(p)
	c.Assert(err, jc.ErrorIsNil)

	b, err := s.storage.GetSecretBackend("myvault")
	c.Assert(err, jc.ErrorIsNil)

	u := state.UpdateSecretBackendParams{
		ID:         b.ID,
		NameChange: ptr("myvault2"),
		Config:     map[string]interface{}{"foo": "bar2"},
	}
	err = s.storage.UpdateSecretBackend(u)
	c.Assert(err, jc.Satisfies, errors.IsAlreadyExists)
}

func (s *SecretBackendsSuite) TestUpdateResetRotationInterval(c *gc.C) {
	p := state.CreateSecretBackendParams{
		Name:                "myvault",
		BackendType:         "vault",
		TokenRotateInterval: ptr(666 * time.Second),
		Config:              map[string]interface{}{"foo.key": "bar"},
	}
	err := s.storage.CreateSecretBackend(p)
	c.Assert(err, jc.ErrorIsNil)
	b, err := s.storage.GetSecretBackend("myvault")
	c.Assert(err, jc.ErrorIsNil)

	u := state.UpdateSecretBackendParams{
		ID:                  b.ID,
		TokenRotateInterval: ptr(0 * time.Second),
		Config:              map[string]interface{}{"foo": "bar2"},
	}
	err = s.storage.UpdateSecretBackend(u)
	c.Assert(err, jc.ErrorIsNil)
	b, err = s.storage.GetSecretBackend("myvault")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(b, jc.DeepEquals, &secrets.SecretBackend{
		ID:          b.ID,
		Name:        "myvault",
		BackendType: "vault",
		Config:      map[string]interface{}{"foo": "bar2"},
	})
}

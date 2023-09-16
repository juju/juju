// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"sort"
	"time"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/testing"
)

type SecretBackendsSuite struct {
	testing.StateSuite
	storage state.SecretBackendsStorage
	store   state.SecretsStore
}

var _ = gc.Suite(&SecretBackendsSuite{})

func (s *SecretBackendsSuite) SetUpTest(c *gc.C) {
	s.StateSuite.SetUpTest(c)
	s.storage = state.NewSecretBackends(s.State)
	s.store = state.NewSecrets(s.State)
}

func (s *SecretBackendsSuite) TestCreate(c *gc.C) {
	now := s.Clock.Now().Round(time.Second).UTC()
	next := now.Add(time.Minute).Round(time.Second).UTC()
	config := map[string]interface{}{"foo.key": "bar"}
	p := state.CreateSecretBackendParams{
		Name:                "myvault",
		BackendType:         "vault",
		TokenRotateInterval: ptr(666 * time.Minute),
		NextRotateTime:      ptr(next),
		Config:              config,
	}
	id, err := s.storage.CreateSecretBackend(p)
	c.Assert(id, gc.Not(gc.Equals), "")
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
	name, nextTime := state.GetSecretBackendNextRotateInfo(c, s.State, id)
	c.Assert(name, gc.Equals, "myvault")
	c.Assert(nextTime, gc.Equals, next)

	_, err = s.storage.CreateSecretBackend(p)
	c.Assert(err, jc.ErrorIs, errors.AlreadyExists)

	p.Name = "another"
	p.ID = id
	_, err = s.storage.CreateSecretBackend(p)
	c.Assert(err, jc.ErrorIs, errors.AlreadyExists)
}

func (s *SecretBackendsSuite) TestGetNotFound(c *gc.C) {
	_, err := s.storage.GetSecretBackend("myvault")
	c.Check(err, jc.ErrorIs, errors.NotFound)
}

func (s *SecretBackendsSuite) TestList(c *gc.C) {
	now := s.Clock.Now().Round(time.Second).UTC()
	next := now.Add(time.Minute).Round(time.Second).UTC()
	config := map[string]interface{}{"foo.key": "bar"}
	p := state.CreateSecretBackendParams{
		Name:                "myvault",
		BackendType:         "vault",
		TokenRotateInterval: ptr(666 * time.Minute),
		NextRotateTime:      ptr(next),
		Config:              config,
	}
	_, err := s.storage.CreateSecretBackend(p)
	c.Assert(err, jc.ErrorIsNil)
	p2 := state.CreateSecretBackendParams{
		Name:        "myk8s",
		BackendType: "kubernetes",
		Config:      config,
	}
	_, err = s.storage.CreateSecretBackend(p2)
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
	_, err := s.storage.CreateSecretBackend(p)
	c.Assert(err, jc.ErrorIsNil)

	err = s.storage.DeleteSecretBackend("myvault", false)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.storage.GetSecretBackend("myvault")
	c.Check(err, jc.ErrorIs, errors.NotFound)
	err = s.storage.DeleteSecretBackend("myvault", false)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *SecretBackendsSuite) TestRemoveWithRevisionsFails(c *gc.C) {
	p := state.CreateSecretBackendParams{
		Name:        "myvault",
		BackendType: "vault",
	}
	_, err := s.storage.CreateSecretBackend(p)
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
	c.Assert(err, jc.ErrorIs, errors.NotSupported)
	count, err := state.SecretBackendRefCount(s.State, b.ID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(count, gc.Equals, 1)
}

func (s *SecretBackendsSuite) TestRemoveWithRevisionsForce(c *gc.C) {
	p := state.CreateSecretBackendParams{
		Name:        "myvault",
		BackendType: "vault",
	}
	_, err := s.storage.CreateSecretBackend(p)
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
	c.Assert(err, jc.ErrorIs, errors.NotFound)
	_, err = s.storage.GetSecretBackend("myvault")
	c.Check(err, jc.ErrorIs, errors.NotFound)
}

func (s *SecretBackendsSuite) TestDeleteSecretUpdatesRefCount(c *gc.C) {
	p := state.CreateSecretBackendParams{
		Name:        "myvault",
		BackendType: "vault",
	}
	_, err := s.storage.CreateSecretBackend(p)
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
	_, err := s.storage.CreateSecretBackend(p)
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
	_, err := s.storage.CreateSecretBackend(p)
	c.Assert(err, jc.ErrorIsNil)
	b, err := s.storage.GetSecretBackend("myvault")
	c.Assert(err, jc.ErrorIsNil)

	now := s.Clock.Now().Round(time.Second).UTC()
	next := now.Add(time.Minute).Round(time.Second).UTC()
	u := state.UpdateSecretBackendParams{
		ID:                  b.ID,
		TokenRotateInterval: ptr(666 * time.Second),
		NextRotateTime:      ptr(next),
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
	name, nextTime := state.GetSecretBackendNextRotateInfo(c, s.State, b.ID)
	c.Assert(name, gc.Equals, "myvault")
	c.Assert(nextTime, gc.Equals, next)
}

func (s *SecretBackendsSuite) TestUpdateName(c *gc.C) {
	now := s.Clock.Now().Round(time.Second).UTC()
	next := now.Add(time.Minute).Round(time.Second).UTC()
	p := state.CreateSecretBackendParams{
		Name:                "myvault",
		BackendType:         "vault",
		TokenRotateInterval: ptr(666 * time.Second),
		NextRotateTime:      ptr(next),
		Config:              map[string]interface{}{"foo.key": "bar"},
	}
	_, err := s.storage.CreateSecretBackend(p)
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
		ID:                  b.ID,
		Name:                "myvault2",
		BackendType:         "vault",
		TokenRotateInterval: ptr(666 * time.Second),
		Config:              map[string]interface{}{"foo": "bar2"},
	})
	name, nextTime := state.GetSecretBackendNextRotateInfo(c, s.State, b.ID)
	c.Assert(name, gc.Equals, "myvault2")
	c.Assert(nextTime, gc.Equals, next)
}

func (s *SecretBackendsSuite) TestUpdateNameForInUseBackend(c *gc.C) {
	now := s.Clock.Now().Round(time.Second).UTC()
	next := now.Add(time.Minute).Round(time.Second).UTC()
	p := state.CreateSecretBackendParams{
		Name:                "myvault",
		BackendType:         "vault",
		TokenRotateInterval: ptr(666 * time.Second),
		NextRotateTime:      ptr(next),
		Config:              map[string]interface{}{"foo.key": "bar"},
	}
	_, err := s.storage.CreateSecretBackend(p)
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
			ValueRef:    &secrets.ValueRef{BackendID: b.ID},
		},
	}
	_, err = s.store.CreateSecret(uri, cp)
	c.Assert(err, jc.ErrorIsNil)

	u := state.UpdateSecretBackendParams{
		ID:         b.ID,
		NameChange: ptr("myvault2"),
		Config:     map[string]interface{}{"foo": "bar2"},
	}
	err = s.storage.UpdateSecretBackend(u)
	c.Assert(err, gc.ErrorMatches, `cannot rename a secret backend that is in use`)
}

func (s *SecretBackendsSuite) TestUpdateNameDuplicate(c *gc.C) {
	p := state.CreateSecretBackendParams{
		Name:        "myvault",
		BackendType: "vault",
		Config:      map[string]interface{}{"foo.key": "bar"},
	}
	_, err := s.storage.CreateSecretBackend(p)
	c.Assert(err, jc.ErrorIsNil)
	p.Name = "myvault2"
	_, err = s.storage.CreateSecretBackend(p)
	c.Assert(err, jc.ErrorIsNil)

	b, err := s.storage.GetSecretBackend("myvault")
	c.Assert(err, jc.ErrorIsNil)

	u := state.UpdateSecretBackendParams{
		ID:         b.ID,
		NameChange: ptr("myvault2"),
		Config:     map[string]interface{}{"foo": "bar2"},
	}
	err = s.storage.UpdateSecretBackend(u)
	c.Assert(err, jc.ErrorIs, errors.AlreadyExists)
}

func (s *SecretBackendsSuite) TestUpdateResetRotationInterval(c *gc.C) {
	now := s.Clock.Now().Round(time.Second).UTC()
	next := now.Add(time.Minute).Round(time.Second).UTC()
	p := state.CreateSecretBackendParams{
		Name:                "myvault",
		BackendType:         "vault",
		TokenRotateInterval: ptr(666 * time.Second),
		NextRotateTime:      ptr(next),
		Config:              map[string]interface{}{"foo.key": "bar"},
	}
	_, err := s.storage.CreateSecretBackend(p)
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

func (s *SecretBackendsSuite) TestSecretBackendRotated(c *gc.C) {
	config := map[string]interface{}{"foo.key": "bar"}
	now := s.Clock.Now().Round(time.Second).UTC()
	next := now.Add(time.Minute).Round(time.Second).UTC()
	cp := state.CreateSecretBackendParams{
		Name:                "myvault",
		BackendType:         "vault",
		TokenRotateInterval: ptr(666 * time.Minute),
		NextRotateTime:      ptr(next),
		Config:              config,
	}
	id, err := s.storage.CreateSecretBackend(cp)
	c.Assert(err, jc.ErrorIsNil)
	next2 := now.Add(time.Hour).Round(time.Second).UTC()
	err = s.storage.SecretBackendRotated(id, next2)
	c.Assert(err, jc.ErrorIsNil)

	_, nextTime := state.GetSecretBackendNextRotateInfo(c, s.State, id)
	c.Assert(nextTime, gc.Equals, next2)
}

func (s *SecretBackendsSuite) TestSecretBackendRotatedConcurrent(c *gc.C) {
	config := map[string]interface{}{"foo.key": "bar"}
	now := s.Clock.Now().Round(time.Second).UTC()
	next := now.Add(time.Minute).Round(time.Second).UTC()
	cp := state.CreateSecretBackendParams{
		Name:                "myvault",
		BackendType:         "vault",
		TokenRotateInterval: ptr(666 * time.Minute),
		NextRotateTime:      ptr(next),
		Config:              config,
	}
	id, err := s.storage.CreateSecretBackend(cp)
	c.Assert(err, jc.ErrorIsNil)

	later := now.Add(time.Hour).Round(time.Second).UTC()
	later2 := now.Add(2 * time.Hour).Round(time.Second).UTC()
	state.SetBeforeHooks(c, s.State, func() {
		err := s.storage.SecretBackendRotated(id, later)
		c.Assert(err, jc.ErrorIsNil)
	})

	err = s.storage.SecretBackendRotated(id, later2)
	c.Assert(err, jc.ErrorIsNil)

	_, nextTime := state.GetSecretBackendNextRotateInfo(c, s.State, id)
	c.Assert(nextTime, gc.Equals, later)
}

type SecretBackendWatcherSuite struct {
	testing.StateSuite
	storage state.SecretBackendsStorage
}

var _ = gc.Suite(&SecretBackendWatcherSuite{})

func (s *SecretBackendWatcherSuite) SetUpTest(c *gc.C) {
	s.StateSuite.SetUpTest(c)
	s.storage = state.NewSecretBackends(s.State)
}

func (s *SecretBackendWatcherSuite) setupWatcher(c *gc.C) (state.SecretBackendRotateWatcher, string) {
	now := s.Clock.Now().Round(time.Second).UTC()
	next := now.Add(time.Minute).Round(time.Second).UTC()
	cp := state.CreateSecretBackendParams{
		Name:                "myvault",
		BackendType:         "vault",
		TokenRotateInterval: ptr(666 * time.Minute),
		NextRotateTime:      ptr(next),
		Config:              map[string]interface{}{"foo.key": "bar"},
	}
	id, err := s.storage.CreateSecretBackend(cp)
	c.Assert(err, jc.ErrorIsNil)
	w, err := s.State.WatchSecretBackendRotationChanges()
	c.Assert(err, jc.ErrorIsNil)

	wc := testing.NewSecretBackendRotateWatcherC(c, w)
	wc.AssertChange(watcher.SecretBackendRotateChange{
		ID:              id,
		Name:            "myvault",
		NextTriggerTime: next,
	})
	wc.AssertNoChange()
	return w, id
}

func (s *SecretBackendWatcherSuite) TestWatchInitialEvent(c *gc.C) {
	w, _ := s.setupWatcher(c)
	workertest.CleanKill(c, w)
}

func (s *SecretBackendWatcherSuite) TestWatchSingleUpdate(c *gc.C) {
	w, id := s.setupWatcher(c)
	wc := testing.NewSecretBackendRotateWatcherC(c, w)
	defer workertest.CleanKill(c, w)

	now := s.Clock.Now().Round(time.Second).UTC()
	next := now.Add(2 * time.Hour).Round(time.Second).UTC()
	err := s.storage.SecretBackendRotated(id, next)
	c.Assert(err, jc.ErrorIsNil)

	wc.AssertChange(watcher.SecretBackendRotateChange{
		ID:              id,
		Name:            "myvault",
		NextTriggerTime: next,
	})
	wc.AssertNoChange()
}

func (s *SecretBackendWatcherSuite) TestWatchDelete(c *gc.C) {
	w, id := s.setupWatcher(c)
	wc := testing.NewSecretBackendRotateWatcherC(c, w)
	defer workertest.CleanKill(c, w)

	err := s.storage.UpdateSecretBackend(state.UpdateSecretBackendParams{
		ID:                  id,
		TokenRotateInterval: ptr(0 * time.Second),
	})
	c.Assert(err, jc.ErrorIsNil)

	wc.AssertChange(watcher.SecretBackendRotateChange{
		ID:   id,
		Name: "myvault",
	})
	wc.AssertNoChange()
}

func (s *SecretBackendWatcherSuite) TestWatchMultipleUpdatesSameBackend(c *gc.C) {
	w, id := s.setupWatcher(c)
	wc := testing.NewSecretBackendRotateWatcherC(c, w)
	defer workertest.CleanKill(c, w)

	// TODO(quiescence): these two changes should be one event.
	now := s.Clock.Now().Round(time.Second).UTC()
	next := now.Add(time.Minute).Round(time.Second).UTC()
	err := s.storage.SecretBackendRotated(id, next)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange(watcher.SecretBackendRotateChange{
		ID:              id,
		Name:            "myvault",
		NextTriggerTime: next,
	})
	next2 := now.Add(time.Hour).Round(time.Second).UTC()
	err = s.storage.SecretBackendRotated(id, next2)
	c.Assert(err, jc.ErrorIsNil)

	wc.AssertChange(watcher.SecretBackendRotateChange{
		ID:              id,
		Name:            "myvault",
		NextTriggerTime: next2,
	})
	wc.AssertNoChange()
}

func (s *SecretBackendWatcherSuite) TestWatchMultipleUpdatesSameBackendDeleted(c *gc.C) {
	w, id := s.setupWatcher(c)
	wc := testing.NewSecretBackendRotateWatcherC(c, w)
	defer workertest.CleanKill(c, w)

	// TODO(quiescence): these two changes should be one event.
	now := s.Clock.Now().Round(time.Second).UTC()
	next := now.Add(time.Hour).Round(time.Second).UTC()
	err := s.storage.SecretBackendRotated(id, next)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange(watcher.SecretBackendRotateChange{
		ID:              id,
		Name:            "myvault",
		NextTriggerTime: next,
	})
	err = s.storage.UpdateSecretBackend(state.UpdateSecretBackendParams{
		ID:                  id,
		TokenRotateInterval: ptr(time.Duration(0)),
	})
	c.Assert(err, jc.ErrorIsNil)

	wc.AssertChange(watcher.SecretBackendRotateChange{
		ID:   id,
		Name: "myvault",
	})
	wc.AssertNoChange()
}

func (s *SecretBackendWatcherSuite) TestWatchMultipleUpdates(c *gc.C) {
	w, id := s.setupWatcher(c)
	wc := testing.NewSecretBackendRotateWatcherC(c, w)
	defer workertest.CleanKill(c, w)

	// TODO(quiescence): these two changes should be one event.
	now := s.Clock.Now().Round(time.Second).UTC()
	next := now.Add(time.Hour).Round(time.Second).UTC()
	err := s.storage.SecretBackendRotated(id, next)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange(watcher.SecretBackendRotateChange{
		ID:              id,
		Name:            "myvault",
		NextTriggerTime: next,
	})

	next2 := now.Add(time.Minute).Round(time.Second).UTC()
	id2, err := s.storage.CreateSecretBackend(state.CreateSecretBackendParams{
		Name:                "myvault2",
		BackendType:         "vault",
		TokenRotateInterval: ptr(666 * time.Minute),
		NextRotateTime:      ptr(next2),
		Config:              map[string]interface{}{"foo.key": "bar"},
	})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange(watcher.SecretBackendRotateChange{
		ID:              id2,
		Name:            "myvault2",
		NextTriggerTime: next2,
	})

	err = s.storage.UpdateSecretBackend(state.UpdateSecretBackendParams{
		ID:                  id,
		TokenRotateInterval: ptr(time.Duration(0)),
	})
	c.Assert(err, jc.ErrorIsNil)

	wc.AssertChange(watcher.SecretBackendRotateChange{
		ID:   id,
		Name: "myvault",
	})
	wc.AssertNoChange()
}

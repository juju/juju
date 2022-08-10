// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"time"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/testing"
)

type SecretsSuite struct {
	testing.StateSuite
	store state.SecretsStore
}

var _ = gc.Suite(&SecretsSuite{})

func (s *SecretsSuite) SetUpTest(c *gc.C) {
	s.StateSuite.SetUpTest(c)
	s.store = state.NewSecretsStore(s.State)
}

func ptr[T any](v T) *T {
	return &v
}

func (s *SecretsSuite) TestCreate(c *gc.C) {
	uri := secrets.NewURI()
	uri.ControllerUUID = s.State.ControllerUUID()
	now := s.Clock.Now().Round(time.Second).UTC()
	p := state.CreateSecretParams{
		Version: 1,
		Owner:   "application-mariadb",
		UpdateSecretParams: state.UpdateSecretParams{
			RotatePolicy:   ptr(secrets.RotateDaily),
			NextRotateTime: ptr(now.Add(time.Minute)),
			Description:    ptr("my secret"),
			Label:          ptr("foobar"),
			ExpireTime:     ptr(now.Add(time.Hour)),
			Params:         nil,
			Data:           map[string]string{"foo": "bar"},
		},
	}
	md, err := s.store.CreateSecret(uri, p)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(md.URI.String(), gc.Equals, uri.String())
	md.URI = nil
	c.Assert(md, jc.DeepEquals, &secrets.SecretMetadata{
		Version:        1,
		Description:    "my secret",
		Label:          "foobar",
		RotatePolicy:   secrets.RotateDaily,
		NextRotateTime: ptr(now.Add(time.Minute)),
		ExpireTime:     ptr(now.Add(time.Hour)),
		OwnerTag:       "application-mariadb",
		ProviderID:     "",
		Revision:       1,
		CreateTime:     now,
		UpdateTime:     now,
	})

	_, err = s.store.CreateSecret(uri, p)
	c.Assert(err, jc.Satisfies, errors.IsAlreadyExists)
}

func (s *SecretsSuite) TestGetValueNotFound(c *gc.C) {
	uri, _ := secrets.ParseURI("secret:9m4e2mr0ui3e8a215n4g")
	_, err := s.store.GetSecretValue(uri, 666)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *SecretsSuite) TestGetValue(c *gc.C) {
	uri := secrets.NewURI()
	uri.ControllerUUID = s.State.ControllerUUID()
	p := state.CreateSecretParams{
		Version:       1,
		ProviderLabel: "juju",
		UpdateSecretParams: state.UpdateSecretParams{
			Data: map[string]string{"foo": "bar"},
		},
	}
	md, err := s.store.CreateSecret(uri, p)
	c.Assert(err, jc.ErrorIsNil)

	val, err := s.store.GetSecretValue(md.URI, 1)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(val.EncodedValues(), jc.DeepEquals, map[string]string{
		"foo": "bar",
	})
}

func (s *SecretsSuite) TestList(c *gc.C) {
	uri := secrets.NewURI()
	uri.ControllerUUID = s.State.ControllerUUID()
	now := s.Clock.Now().Round(time.Second).UTC()
	p := state.CreateSecretParams{
		Version:       1,
		ProviderLabel: "juju",
		UpdateSecretParams: state.UpdateSecretParams{
			RotatePolicy:   ptr(secrets.RotateDaily),
			NextRotateTime: ptr(now.Add(time.Minute)),
			Description:    ptr("my secret"),
			Label:          ptr("foobar"),
			ExpireTime:     ptr(now.Add(time.Hour)),
			Params:         nil,
			Data:           map[string]string{"foo": "bar"},
		},
	}
	_, err := s.store.CreateSecret(uri, p)
	c.Assert(err, jc.ErrorIsNil)

	list, err := s.store.ListSecrets(state.SecretsFilter{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(list, jc.DeepEquals, []*secrets.SecretMetadata{{
		URI:            uri,
		RotatePolicy:   secrets.RotateDaily,
		NextRotateTime: ptr(now.Add(time.Minute)),
		ExpireTime:     ptr(now.Add(time.Hour)),
		Version:        1,
		Description:    "my secret",
		Label:          "foobar",
		Provider:       "juju",
		ProviderID:     "",
		Revision:       1,
		CreateTime:     now,
		UpdateTime:     now,
	}})
}

func (s *SecretsSuite) TestUpdateNothing(c *gc.C) {
	up := state.UpdateSecretParams{}
	uri := secrets.NewURI()
	_, err := s.store.UpdateSecret(uri, up)
	c.Assert(err, gc.ErrorMatches, "must specify a new value or metadata to update a secret")
}

func (s *SecretsSuite) TestUpdateAll(c *gc.C) {
	uri := secrets.NewURI()
	uri.ControllerUUID = s.State.ControllerUUID()
	now := s.Clock.Now().Round(time.Second).UTC()
	cp := state.CreateSecretParams{
		Version:       1,
		ProviderLabel: "juju",
		UpdateSecretParams: state.UpdateSecretParams{
			RotatePolicy:   ptr(secrets.RotateDaily),
			NextRotateTime: ptr(now.Add(time.Minute)),
			Description:    ptr("my secret"),
			Label:          ptr("foobar"),
			Data:           map[string]string{"foo": "bar"},
		},
	}
	md, err := s.store.CreateSecret(uri, cp)
	c.Assert(err, jc.ErrorIsNil)
	newData := map[string]string{"foo": "bar", "hello": "world"}
	s.assertUpdatedSecret(c, md, 2, state.UpdateSecretParams{
		Description:    ptr("big secret"),
		Label:          ptr("new label"),
		RotatePolicy:   ptr(secrets.RotateHourly),
		NextRotateTime: ptr(now.Add(2 * time.Minute)),
		Data:           newData,
	})
}

func (s *SecretsSuite) TestUpdateRotateInterval(c *gc.C) {
	uri := secrets.NewURI()
	uri.ControllerUUID = s.State.ControllerUUID()
	now := s.Clock.Now().Round(time.Second).UTC()
	cp := state.CreateSecretParams{
		Version:       1,
		ProviderLabel: "juju",
		UpdateSecretParams: state.UpdateSecretParams{
			RotatePolicy:   ptr(secrets.RotateDaily),
			NextRotateTime: ptr(now.Add(time.Minute)),
			Data:           map[string]string{"foo": "bar"},
		},
	}
	md, err := s.store.CreateSecret(uri, cp)
	c.Assert(err, jc.ErrorIsNil)
	s.assertUpdatedSecret(c, md, 1, state.UpdateSecretParams{
		RotatePolicy:   ptr(secrets.RotateHourly),
		NextRotateTime: ptr(now.Add(time.Minute)),
	})
}

func (s *SecretsSuite) TestUpdateData(c *gc.C) {
	uri := secrets.NewURI()
	uri.ControllerUUID = s.State.ControllerUUID()
	now := s.Clock.Now().Round(time.Second).UTC()
	cp := state.CreateSecretParams{
		ProviderLabel: "juju",
		Version:       1,
		UpdateSecretParams: state.UpdateSecretParams{
			RotatePolicy:   ptr(secrets.RotateDaily),
			NextRotateTime: ptr(now.Add(time.Minute)),
			Data:           map[string]string{"foo": "bar"},
		},
	}
	md, err := s.store.CreateSecret(uri, cp)
	c.Assert(err, jc.ErrorIsNil)
	newData := map[string]string{"foo": "bar", "hello": "world"}
	s.assertUpdatedSecret(c, md, 2, state.UpdateSecretParams{
		Data: newData,
	})

}

func (s *SecretsSuite) assertUpdatedSecret(c *gc.C, original *secrets.SecretMetadata, expectedRevision int, update state.UpdateSecretParams) {
	expected := *original
	expected.Revision = expectedRevision
	if update.RotatePolicy != nil {
		expected.RotatePolicy = *update.RotatePolicy
		expected.NextRotateTime = update.NextRotateTime
	}
	if update.Description != nil {
		expected.Description = *update.Description
	}
	if update.Label != nil {
		expected.Label = *update.Label
	}

	s.Clock.Advance(time.Hour)
	updated := s.Clock.Now().Round(time.Second).UTC()
	expected.UpdateTime = updated
	md, err := s.store.UpdateSecret(original.URI, update)
	c.Assert(err, jc.ErrorIsNil)

	list, err := s.store.ListSecrets(state.SecretsFilter{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(list, jc.DeepEquals, []*secrets.SecretMetadata{&expected})
	expectedData := map[string]string{"foo": "bar"}
	if update.Data != nil {
		expectedData = update.Data
	}
	val, err := s.store.GetSecretValue(md.URI, expectedRevision)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(val.EncodedValues(), jc.DeepEquals, expectedData)
}

func (s *SecretsSuite) TestUpdateConcurrent(c *gc.C) {
	uri := secrets.NewURI()
	uri.ControllerUUID = s.State.ControllerUUID()

	now := s.Clock.Now().Round(time.Second).UTC()
	cp := state.CreateSecretParams{
		Version:       1,
		ProviderLabel: "juju",
		UpdateSecretParams: state.UpdateSecretParams{
			RotatePolicy:   ptr(secrets.RotateDaily),
			NextRotateTime: ptr(now.Add(time.Minute)),
			Data:           map[string]string{"foo": "bar"},
		},
	}
	md, err := s.store.CreateSecret(uri, cp)

	state.SetBeforeHooks(c, s.State, func() {
		up := state.UpdateSecretParams{
			RotatePolicy:   ptr(secrets.RotateYearly),
			NextRotateTime: ptr(now.Add(time.Minute)),
			Params:         nil,
			Data:           map[string]string{"foo": "baz", "goodbye": "world"},
		}
		md, err = s.store.UpdateSecret(md.URI, up)
		c.Assert(err, jc.ErrorIsNil)
	})
	newData := map[string]string{"foo": "bar", "hello": "world"}
	s.assertUpdatedSecret(c, md, 3, state.UpdateSecretParams{
		RotatePolicy:   ptr(secrets.RotateHourly),
		NextRotateTime: ptr(now.Add(time.Minute)),
		Data:           newData,
	})
}

func (s *SecretsSuite) TestGetSecretConsumer(c *gc.C) {
	uri := secrets.NewURI()
	uri.ControllerUUID = s.State.ControllerUUID()
	_, err := s.store.GetSecretConsumer(uri, "application-mariadb")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	md := &secrets.SecretConsumerMetadata{
		Label:    "foobar",
		Revision: 666,
	}
	err = s.store.SaveSecretConsumer(uri, "application-mariadb", md)
	c.Assert(err, jc.ErrorIsNil)
	md2, err := s.store.GetSecretConsumer(uri, "application-mariadb")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(md2, jc.DeepEquals, md)
	_, err = s.store.GetSecretConsumer(uri, "application-mysql")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *SecretsSuite) TestSaveSecretConsumer(c *gc.C) {
	uri := secrets.NewURI()
	uri.ControllerUUID = s.State.ControllerUUID()
	md := &secrets.SecretConsumerMetadata{
		Label:    "foobar",
		Revision: 666,
	}
	err := s.store.SaveSecretConsumer(uri, "application-mariadb", md)
	c.Assert(err, jc.ErrorIsNil)
	md2, err := s.store.GetSecretConsumer(uri, "application-mariadb")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(md2, jc.DeepEquals, md)
	md.Revision = 668
	err = s.store.SaveSecretConsumer(uri, "application-mariadb", md)
	c.Assert(err, jc.ErrorIsNil)
	md2, err = s.store.GetSecretConsumer(uri, "application-mariadb")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(md2, jc.DeepEquals, md)
}

func (s *SecretsSuite) TestSaveSecretConsumerConcurrent(c *gc.C) {
	uri := secrets.NewURI()
	uri.ControllerUUID = s.State.ControllerUUID()
	md := &secrets.SecretConsumerMetadata{
		Label:    "foobar",
		Revision: 666,
	}
	state.SetBeforeHooks(c, s.State, func() {
		err := s.store.SaveSecretConsumer(uri, "application-mariadb", &secrets.SecretConsumerMetadata{Revision: 668})
		c.Assert(err, jc.ErrorIsNil)
	})
	err := s.store.SaveSecretConsumer(uri, "application-mariadb", md)
	c.Assert(err, jc.ErrorIsNil)
	md2, err := s.store.GetSecretConsumer(uri, "application-mariadb")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(md2, jc.DeepEquals, md)
}

func (s *SecretsSuite) TestSecretRotated(c *gc.C) {
	uri := secrets.NewURI()
	uri.ControllerUUID = s.State.ControllerUUID()

	now := s.Clock.Now().Round(time.Second).UTC()
	cp := state.CreateSecretParams{
		Version:       1,
		ProviderLabel: "juju",
		UpdateSecretParams: state.UpdateSecretParams{
			RotatePolicy:   ptr(secrets.RotateDaily),
			NextRotateTime: ptr(now.Add(time.Minute)),
			Data:           map[string]string{"foo": "bar"},
		},
	}
	md, err := s.store.CreateSecret(uri, cp)
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.SecretRotated(uri, now)
	c.Assert(err, jc.ErrorIsNil)

	rotated := state.GetSecretRotateTime(c, s.State, md.URI.ID)
	c.Assert(rotated, gc.Equals, now.Round(time.Second).UTC())
}

func (s *SecretsSuite) TestSecretRotatedConcurrent(c *gc.C) {
	uri := secrets.NewURI()
	uri.ControllerUUID = s.State.ControllerUUID()

	now := s.Clock.Now().Round(time.Second).UTC()
	cp := state.CreateSecretParams{
		Version:       1,
		ProviderLabel: "juju",
		UpdateSecretParams: state.UpdateSecretParams{
			RotatePolicy:   ptr(secrets.RotateDaily),
			NextRotateTime: ptr(now.Add(time.Minute)),
			Data:           map[string]string{"foo": "bar"},
		},
	}
	md, err := s.store.CreateSecret(uri, cp)
	c.Assert(err, jc.ErrorIsNil)

	later := now.Add(time.Hour)
	state.SetBeforeHooks(c, s.State, func() {
		err := s.State.SecretRotated(uri, later)
		c.Assert(err, jc.ErrorIsNil)
	})

	err = s.State.SecretRotated(uri, now)
	c.Assert(err, jc.ErrorIsNil)

	rotated := state.GetSecretRotateTime(c, s.State, md.URI.ID)
	c.Assert(rotated, gc.Equals, later.Round(time.Second))
}

type SecretsWatcherSuite struct {
	testing.StateSuite
	store state.SecretsStore
}

var _ = gc.Suite(&SecretsWatcherSuite{})

func (s *SecretsWatcherSuite) SetUpTest(c *gc.C) {
	c.Skip("rotation not implemented")
	s.StateSuite.SetUpTest(c)
	s.store = state.NewSecretsStore(s.State)
}

func (s *SecretsWatcherSuite) setupWatcher(c *gc.C) (state.SecretsRotationWatcher, *secrets.URI) {
	uri := secrets.NewURI()
	now := s.Clock.Now().Round(time.Second).UTC()
	cp := state.CreateSecretParams{
		Version: 1,
		UpdateSecretParams: state.UpdateSecretParams{
			RotatePolicy:   ptr(secrets.RotateDaily),
			NextRotateTime: ptr(now.Add(time.Hour)),
			Data:           map[string]string{"foo": "bar"},
		},
	}
	md, err := s.store.CreateSecret(uri, cp)
	c.Assert(err, jc.ErrorIsNil)
	w := s.State.WatchSecretsRotationChanges("application-mariadb")

	wc := testing.NewSecretsRotationWatcherC(c, s.State, w)
	wc.AssertChange(watcher.SecretRotationChange{
		URI:            md.URI.Raw(),
		RotateInterval: time.Hour,
		LastRotateTime: now,
	})
	wc.AssertNoChange()
	return w, uri
}

func (s *SecretsWatcherSuite) TestWatchInitialEvent(c *gc.C) {
	w, _ := s.setupWatcher(c)
	testing.AssertStop(c, w)
}

func (s *SecretsWatcherSuite) TestWatchSingleUpdate(c *gc.C) {
	w, uri := s.setupWatcher(c)
	wc := testing.NewSecretsRotationWatcherC(c, s.State, w)
	defer testing.AssertStop(c, w)

	now := s.Clock.Now().Round(time.Second).UTC()
	md, err := s.store.UpdateSecret(uri, state.UpdateSecretParams{
		RotatePolicy:   ptr(secrets.RotateHourly),
		NextRotateTime: ptr(now.Add(time.Minute)),
	})
	c.Assert(err, jc.ErrorIsNil)

	wc.AssertChange(watcher.SecretRotationChange{
		URI:            md.URI.Raw(),
		RotateInterval: time.Hour,
		LastRotateTime: md.CreateTime.UTC(),
	})
	wc.AssertNoChange()
}

func (s *SecretsWatcherSuite) TestWatchDelete(c *gc.C) {
	w, uri := s.setupWatcher(c)
	wc := testing.NewSecretsRotationWatcherC(c, s.State, w)
	defer testing.AssertStop(c, w)

	md, err := s.store.UpdateSecret(uri, state.UpdateSecretParams{
		RotatePolicy: ptr(secrets.RotateNever),
	})
	c.Assert(err, jc.ErrorIsNil)

	wc.AssertChange(watcher.SecretRotationChange{
		URI:            md.URI.Raw(),
		RotateInterval: 0,
	})
	wc.AssertNoChange()
}

func (s *SecretsWatcherSuite) TestWatchMultipleUpdatesSameSecret(c *gc.C) {
	w, uri := s.setupWatcher(c)
	wc := testing.NewSecretsRotationWatcherC(c, s.State, w)
	defer testing.AssertStop(c, w)

	now := s.Clock.Now().Round(time.Second).UTC()
	_, err := s.store.UpdateSecret(uri, state.UpdateSecretParams{
		RotatePolicy:   ptr(secrets.RotateYearly),
		NextRotateTime: ptr(now.Add(time.Minute)),
	})
	c.Assert(err, jc.ErrorIsNil)
	md, err := s.store.UpdateSecret(uri, state.UpdateSecretParams{
		RotatePolicy:   ptr(secrets.RotateHourly),
		NextRotateTime: ptr(now.Add(time.Minute)),
	})
	c.Assert(err, jc.ErrorIsNil)

	wc.AssertChange(watcher.SecretRotationChange{
		URI:            md.URI.Raw(),
		RotateInterval: time.Hour,
		LastRotateTime: md.CreateTime.UTC(),
	})
	wc.AssertNoChange()
}

func (s *SecretsWatcherSuite) TestWatchMultipleUpdatesSameSecretDeleted(c *gc.C) {
	w, uri := s.setupWatcher(c)
	wc := testing.NewSecretsRotationWatcherC(c, s.State, w)
	defer testing.AssertStop(c, w)

	now := s.Clock.Now().Round(time.Second).UTC()
	_, err := s.store.UpdateSecret(uri, state.UpdateSecretParams{
		RotatePolicy:   ptr(secrets.RotateHourly),
		NextRotateTime: ptr(now.Add(time.Minute)),
	})
	c.Assert(err, jc.ErrorIsNil)
	md, err := s.store.UpdateSecret(uri, state.UpdateSecretParams{
		RotatePolicy: ptr(secrets.RotateNever),
	})
	c.Assert(err, jc.ErrorIsNil)

	wc.AssertChange(watcher.SecretRotationChange{
		URI:            md.URI.Raw(),
		RotateInterval: 0,
	})
	wc.AssertNoChange()
}

func (s *SecretsWatcherSuite) TestWatchMultipleUpdates(c *gc.C) {
	w, uri := s.setupWatcher(c)
	wc := testing.NewSecretsRotationWatcherC(c, s.State, w)
	defer testing.AssertStop(c, w)

	now := s.Clock.Now().Round(time.Second).UTC()
	_, err := s.store.UpdateSecret(uri, state.UpdateSecretParams{
		RotatePolicy:   ptr(secrets.RotateHourly),
		NextRotateTime: ptr(now.Add(time.Minute)),
	})
	c.Assert(err, jc.ErrorIsNil)

	uri2 := secrets.NewURI()
	md2, err := s.store.CreateSecret(uri2, state.CreateSecretParams{
		Version: 1,
		Owner:   "application-mariadb",
		UpdateSecretParams: state.UpdateSecretParams{
			RotatePolicy:   ptr(secrets.RotateHourly),
			NextRotateTime: ptr(now.Add(time.Minute)),
			Data:           map[string]string{"foo": "bar"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	md, err := s.store.UpdateSecret(uri, state.UpdateSecretParams{
		RotatePolicy: ptr(secrets.RotateNever),
	})
	c.Assert(err, jc.ErrorIsNil)

	wc.AssertChange(watcher.SecretRotationChange{
		URI:            md2.URI.Raw(),
		RotateInterval: time.Hour,
		LastRotateTime: md2.CreateTime.UTC(),
	}, watcher.SecretRotationChange{
		URI:            md.URI.Raw(),
		RotateInterval: 0,
	})
	wc.AssertNoChange()
}

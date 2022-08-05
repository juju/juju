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

func (s *SecretsSuite) TestCreate(c *gc.C) {
	uri := secrets.NewURI()
	uri.ControllerUUID = s.State.ControllerUUID()
	p := state.CreateSecretParams{
		Version:        1,
		RotateInterval: time.Hour,
		Params:         nil,
		Data:           map[string]string{"foo": "bar"},
		Owner:          "application-mariadb",
	}
	md, err := s.store.CreateSecret(uri, p)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(md.URI.String(), gc.Equals, uri.String())
	md.URI = nil
	now := s.Clock.Now().Round(time.Second).UTC()
	c.Assert(md, jc.DeepEquals, &secrets.SecretMetadata{
		RotateInterval: time.Hour,
		Version:        1,
		Description:    "",
		OwnerTag:       "application-mariadb",
		Tags:           nil,
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
		Version:        1,
		ProviderLabel:  "juju",
		RotateInterval: time.Hour,
		Params:         nil,
		Data:           map[string]string{"foo": "bar"},
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
	p := state.CreateSecretParams{
		Version:        1,
		ProviderLabel:  "juju",
		RotateInterval: time.Hour,
		Params:         nil,
		Data:           map[string]string{"foo": "bar"},
	}
	_, err := s.store.CreateSecret(uri, p)
	c.Assert(err, jc.ErrorIsNil)

	list, err := s.store.ListSecrets(state.SecretsFilter{})
	c.Assert(err, jc.ErrorIsNil)
	now := s.Clock.Now().Round(time.Second).UTC()
	c.Assert(list, jc.DeepEquals, []*secrets.SecretMetadata{{
		URI:            uri,
		RotateInterval: time.Hour,
		Version:        1,
		Description:    "",
		Tags:           map[string]string{},
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

func durationPtr(d time.Duration) *time.Duration {
	return &d
}

func (s *SecretsSuite) TestUpdateAll(c *gc.C) {
	uri := secrets.NewURI()
	uri.ControllerUUID = s.State.ControllerUUID()
	cp := state.CreateSecretParams{
		Version:        1,
		ProviderLabel:  "juju",
		RotateInterval: time.Hour,
		Params:         nil,
		Data:           map[string]string{"foo": "bar"},
	}
	md, err := s.store.CreateSecret(uri, cp)
	c.Assert(err, jc.ErrorIsNil)
	newData := map[string]string{"foo": "bar", "hello": "world"}
	newDescription := "big secret"
	newTags := map[string]string{"goodbye": "world"}
	s.assertUpdatedSecret(c, md.URI, newData, durationPtr(2*time.Hour), &newDescription, &newTags, 2)
}

func (s *SecretsSuite) TestUpdateRotateInterval(c *gc.C) {
	uri := secrets.NewURI()
	uri.ControllerUUID = s.State.ControllerUUID()
	cp := state.CreateSecretParams{
		Version:        1,
		ProviderLabel:  "juju",
		RotateInterval: time.Hour,
		Params:         nil,
		Data:           map[string]string{"foo": "bar"},
	}
	md, err := s.store.CreateSecret(uri, cp)
	c.Assert(err, jc.ErrorIsNil)
	s.assertUpdatedSecret(c, md.URI, nil, durationPtr(2*time.Hour), nil, nil, 1)
}

func (s *SecretsSuite) TestUpdateData(c *gc.C) {
	uri := secrets.NewURI()
	uri.ControllerUUID = s.State.ControllerUUID()
	cp := state.CreateSecretParams{
		ProviderLabel:  "juju",
		Version:        1,
		RotateInterval: time.Hour,
		Description:    "my secret",
		Tags:           map[string]string{"hello": "world"},
		Params:         nil,
		Data:           map[string]string{"foo": "bar"},
	}
	md, err := s.store.CreateSecret(uri, cp)
	c.Assert(err, jc.ErrorIsNil)
	newData := map[string]string{"foo": "bar", "hello": "world"}
	s.assertUpdatedSecret(c, md.URI, newData, nil, nil, nil, 2)
}

func (s *SecretsSuite) assertUpdatedSecret(c *gc.C, uri *secrets.URI, data map[string]string, rotateInterval *time.Duration, description *string, tags *map[string]string, expectedRevision int) {
	created := s.Clock.Now().Round(time.Second).UTC()

	up := state.UpdateSecretParams{
		RotateInterval: rotateInterval,
		Description:    description,
		Tags:           tags,
		Params:         nil,
		Data:           data,
	}
	s.Clock.Advance(time.Hour)
	updated := s.Clock.Now().Round(time.Second).UTC()
	md, err := s.store.UpdateSecret(uri, up)
	c.Assert(err, jc.ErrorIsNil)
	expected := &secrets.SecretMetadata{
		URI:            md.URI,
		Version:        1,
		RotateInterval: md.RotateInterval,
		Description:    md.Description,
		Tags:           md.Tags,
		Provider:       "juju",
		ProviderID:     "",
		Revision:       expectedRevision,
		CreateTime:     created,
		UpdateTime:     updated,
	}
	if rotateInterval != nil {
		expected.RotateInterval = *rotateInterval
	}
	if description != nil {
		expected.Description = *description
	}
	if tags != nil {
		expected.Tags = *tags
	}
	c.Assert(md, jc.DeepEquals, expected)

	list, err := s.store.ListSecrets(state.SecretsFilter{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(list, jc.DeepEquals, []*secrets.SecretMetadata{expected})
	expectedData := map[string]string{"foo": "bar"}
	if data != nil {
		expectedData = data
	}
	val, err := s.store.GetSecretValue(md.URI, expectedRevision)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(val.EncodedValues(), jc.DeepEquals, expectedData)
}

func (s *SecretsSuite) TestUpdateConcurrent(c *gc.C) {
	uri := secrets.NewURI()
	uri.ControllerUUID = s.State.ControllerUUID()

	cp := state.CreateSecretParams{
		Version:        1,
		ProviderLabel:  "juju",
		RotateInterval: time.Hour,
		Params:         nil,
		Data:           map[string]string{"foo": "bar"},
	}
	md, err := s.store.CreateSecret(uri, cp)

	state.SetBeforeHooks(c, s.State, func() {
		up := state.UpdateSecretParams{
			RotateInterval: durationPtr(3 * time.Hour),
			Params:         nil,
			Data:           map[string]string{"foo": "baz", "goodbye": "world"},
		}
		md, err = s.store.UpdateSecret(md.URI, up)
		c.Assert(err, jc.ErrorIsNil)
	})
	newData := map[string]string{"foo": "bar", "hello": "world"}
	s.assertUpdatedSecret(c, md.URI, newData, durationPtr(2*time.Hour), nil, nil, 3)
}

func (s *SecretsSuite) TestSecretRotated(c *gc.C) {
	uri := secrets.NewURI()
	uri.ControllerUUID = s.State.ControllerUUID()
	cp := state.CreateSecretParams{
		Version:        1,
		ProviderLabel:  "juju",
		RotateInterval: time.Hour,
		Params:         nil,
		Data:           map[string]string{"foo": "bar"},
	}
	md, err := s.store.CreateSecret(uri, cp)
	c.Assert(err, jc.ErrorIsNil)
	now := time.Now()
	err = s.State.SecretRotated(uri, now)
	c.Assert(err, jc.ErrorIsNil)

	rotated := state.GetSecretRotateTime(c, s.State, md.URI.ID)
	c.Assert(rotated, gc.Equals, now.Round(time.Second))
}

func (s *SecretsSuite) TestSecretRotatedConcurrent(c *gc.C) {
	uri := secrets.NewURI()
	uri.ControllerUUID = s.State.ControllerUUID()
	cp := state.CreateSecretParams{
		Version:        1,
		ProviderLabel:  "juju",
		RotateInterval: time.Hour,
		Params:         nil,
		Data:           map[string]string{"foo": "bar"},
	}
	md, err := s.store.CreateSecret(uri, cp)
	c.Assert(err, jc.ErrorIsNil)

	now := time.Now()
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
	s.StateSuite.SetUpTest(c)
	s.store = state.NewSecretsStore(s.State)
}

func (s *SecretsWatcherSuite) setupWatcher(c *gc.C) (state.SecretsRotationWatcher, *secrets.URI) {
	uri := secrets.NewURI()
	md, err := s.store.CreateSecret(uri, state.CreateSecretParams{
		Version:        1,
		Owner:          "application-mariadb",
		RotateInterval: time.Hour,
	})
	c.Assert(err, jc.ErrorIsNil)
	w := s.State.WatchSecretsRotationChanges("application-mariadb")

	now := s.Clock.Now().Round(time.Second).UTC()
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

	md, err := s.store.UpdateSecret(uri, state.UpdateSecretParams{
		RotateInterval: durationPtr(time.Minute),
	})
	c.Assert(err, jc.ErrorIsNil)

	wc.AssertChange(watcher.SecretRotationChange{
		URI:            md.URI.Raw(),
		RotateInterval: time.Minute,
		LastRotateTime: md.CreateTime.UTC(),
	})
	wc.AssertNoChange()
}

func (s *SecretsWatcherSuite) TestWatchDelete(c *gc.C) {
	w, uri := s.setupWatcher(c)
	wc := testing.NewSecretsRotationWatcherC(c, s.State, w)
	defer testing.AssertStop(c, w)

	md, err := s.store.UpdateSecret(uri, state.UpdateSecretParams{
		RotateInterval: durationPtr(0),
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

	_, err := s.store.UpdateSecret(uri, state.UpdateSecretParams{
		RotateInterval: durationPtr(time.Minute),
	})
	c.Assert(err, jc.ErrorIsNil)
	md, err := s.store.UpdateSecret(uri, state.UpdateSecretParams{
		RotateInterval: durationPtr(time.Second),
	})
	c.Assert(err, jc.ErrorIsNil)

	wc.AssertChange(watcher.SecretRotationChange{
		URI:            md.URI.Raw(),
		RotateInterval: time.Second,
		LastRotateTime: md.CreateTime.UTC(),
	})
	wc.AssertNoChange()
}

func (s *SecretsWatcherSuite) TestWatchMultipleUpdatesSameSecretDeleted(c *gc.C) {
	w, uri := s.setupWatcher(c)
	wc := testing.NewSecretsRotationWatcherC(c, s.State, w)
	defer testing.AssertStop(c, w)

	_, err := s.store.UpdateSecret(uri, state.UpdateSecretParams{
		RotateInterval: durationPtr(time.Minute),
	})
	c.Assert(err, jc.ErrorIsNil)
	md, err := s.store.UpdateSecret(uri, state.UpdateSecretParams{
		RotateInterval: durationPtr(0),
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

	_, err := s.store.UpdateSecret(uri, state.UpdateSecretParams{
		RotateInterval: durationPtr(time.Minute),
	})
	c.Assert(err, jc.ErrorIsNil)

	uri2 := secrets.NewURI()
	md2, err := s.store.CreateSecret(uri2, state.CreateSecretParams{
		Version:        1,
		Owner:          "application-mariadb",
		RotateInterval: time.Hour,
	})
	c.Assert(err, jc.ErrorIsNil)

	md, err := s.store.UpdateSecret(uri, state.UpdateSecretParams{
		RotateInterval: durationPtr(0),
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

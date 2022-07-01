// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"time"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/v3/core/secrets"
	"github.com/juju/juju/v3/core/watcher"
	"github.com/juju/juju/v3/state"
	"github.com/juju/juju/v3/state/testing"
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
	URL := secrets.NewSimpleURL("app/mariadb/password")
	URL.ControllerUUID = s.State.ControllerUUID()
	URL.ModelUUID = s.State.ModelUUID()
	p := state.CreateSecretParams{
		Version:        1,
		Type:           "blob",
		Path:           "app/mariadb/password",
		RotateInterval: time.Hour,
		Params:         nil,
		Data:           map[string]string{"foo": "bar"},
	}
	md, err := s.store.CreateSecret(URL, p)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(md.URL.String(), gc.Equals, URL.String())
	md.URL = nil
	now := s.Clock.Now().Round(time.Second).UTC()
	c.Assert(md, jc.DeepEquals, &secrets.SecretMetadata{
		Path:           p.Path,
		RotateInterval: time.Hour,
		Version:        1,
		Description:    "",
		Tags:           nil,
		ID:             1,
		ProviderID:     "",
		Revision:       1,
		CreateTime:     now,
		UpdateTime:     now,
	})

	_, err = s.store.CreateSecret(URL, p)
	c.Assert(err, jc.Satisfies, errors.IsAlreadyExists)
}

func (s *SecretsSuite) TestCreateIncrementsID(c *gc.C) {
	URL := secrets.NewSimpleURL("app/mariadb/password")
	URL.ControllerUUID = s.State.ControllerUUID()
	URL.ModelUUID = s.State.ModelUUID()
	p := state.CreateSecretParams{
		Version:        1,
		Type:           "blob",
		Path:           "app/mariadb/password",
		RotateInterval: time.Hour,
		Params:         nil,
		Data:           map[string]string{"foo": "bar"},
	}
	_, err := s.store.CreateSecret(URL, p)
	c.Assert(err, jc.ErrorIsNil)

	URL.Path = "app/mariadb/password2"
	p.Path = URL.Path
	md, err := s.store.CreateSecret(URL, p)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(md.URL.String(), gc.Equals, URL.String())
	c.Assert(md.ID, gc.Equals, 2)
}

func (s *SecretsSuite) TestGetValueNotFound(c *gc.C) {
	URL, _ := secrets.ParseURL("secret://app/mariadb/password")
	_, err := s.store.GetSecretValue(URL)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *SecretsSuite) TestGetValue(c *gc.C) {
	URL := secrets.NewSimpleURL("app/mariadb/password")
	URL.ControllerUUID = s.State.ControllerUUID()
	URL.ModelUUID = s.State.ModelUUID()
	p := state.CreateSecretParams{
		Version:        1,
		ProviderLabel:  "juju",
		Type:           "blob",
		Path:           "app/mariadb/password",
		RotateInterval: time.Hour,
		Params:         nil,
		Data:           map[string]string{"foo": "bar"},
	}
	md, err := s.store.CreateSecret(URL, p)
	c.Assert(err, jc.ErrorIsNil)

	val, err := s.store.GetSecretValue(md.URL.WithRevision(1))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(val.EncodedValues(), jc.DeepEquals, map[string]string{
		"foo": "bar",
	})
}

func (s *SecretsSuite) TestGetValueAttribute(c *gc.C) {
	URL := secrets.NewSimpleURL("app/mariadb/password")
	URL.ControllerUUID = s.State.ControllerUUID()
	URL.ModelUUID = s.State.ModelUUID()
	p := state.CreateSecretParams{
		Version:        1,
		ProviderLabel:  "juju",
		Type:           "blob",
		Path:           "app/mariadb/password",
		RotateInterval: time.Hour,
		Params:         nil,
		Data:           map[string]string{"foo": "bar", "hello": "world"},
	}
	md, err := s.store.CreateSecret(URL, p)
	c.Assert(err, jc.ErrorIsNil)

	val, err := s.store.GetSecretValue(md.URL.WithRevision(1).WithAttribute("hello"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(val.EncodedValues(), jc.DeepEquals, map[string]string{
		"hello": "world",
	})
}

func (s *SecretsSuite) TestGetValueAttributeNotFound(c *gc.C) {
	URL := secrets.NewSimpleURL("app/mariadb/password")
	URL.ControllerUUID = s.State.ControllerUUID()
	URL.ModelUUID = s.State.ModelUUID()
	p := state.CreateSecretParams{
		Version:        1,
		ProviderLabel:  "juju",
		Type:           "blob",
		Path:           "app/mariadb/password",
		RotateInterval: time.Hour,
		Params:         nil,
		Data:           map[string]string{"foo": "bar", "hello": "world"},
	}
	md, err := s.store.CreateSecret(URL, p)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.store.GetSecretValue(md.URL.WithRevision(1).WithAttribute("goodbye"))
	c.Assert(err, gc.ErrorMatches, `secret attribute "goodbye" not found`)
}

func (s *SecretsSuite) TestList(c *gc.C) {
	URL := secrets.NewSimpleURL("app/mariadb/password")
	URL.ControllerUUID = s.State.ControllerUUID()
	URL.ModelUUID = s.State.ModelUUID()
	p := state.CreateSecretParams{
		Version:        1,
		ProviderLabel:  "juju",
		Type:           "blob",
		Path:           "app/mariadb/password",
		RotateInterval: time.Hour,
		Params:         nil,
		Data:           map[string]string{"foo": "bar"},
	}
	_, err := s.store.CreateSecret(URL, p)
	c.Assert(err, jc.ErrorIsNil)

	list, err := s.store.ListSecrets(state.SecretsFilter{})
	c.Assert(err, jc.ErrorIsNil)
	now := s.Clock.Now().Round(time.Second).UTC()
	c.Assert(list, jc.DeepEquals, []*secrets.SecretMetadata{{
		URL:            URL,
		Path:           "app/mariadb/password",
		RotateInterval: time.Hour,
		Version:        1,
		Description:    "",
		Tags:           map[string]string{},
		ID:             1,
		Provider:       "juju",
		ProviderID:     "",
		Revision:       1,
		CreateTime:     now,
		UpdateTime:     now,
	}})
}

func (s *SecretsSuite) TestUpdateNothing(c *gc.C) {
	up := state.UpdateSecretParams{}
	URL := secrets.NewSimpleURL("password")
	_, err := s.store.UpdateSecret(URL, up)
	c.Assert(err, gc.ErrorMatches, "must specify a new value or metadata to update a secret")
}

func durationPtr(d time.Duration) *time.Duration {
	return &d
}

func (s *SecretsSuite) TestUpdateAll(c *gc.C) {
	URL := secrets.NewSimpleURL("app/mariadb/password")
	URL.ControllerUUID = s.State.ControllerUUID()
	URL.ModelUUID = s.State.ModelUUID()
	cp := state.CreateSecretParams{
		Version:        1,
		ProviderLabel:  "juju",
		Type:           "blob",
		Path:           "app/mariadb/password",
		RotateInterval: time.Hour,
		Params:         nil,
		Data:           map[string]string{"foo": "bar"},
	}
	md, err := s.store.CreateSecret(URL, cp)
	c.Assert(err, jc.ErrorIsNil)
	newData := map[string]string{"foo": "bar", "hello": "world"}
	newDescription := "big secret"
	newTags := map[string]string{"goodbye": "world"}
	newStatus := secrets.StatusStaged
	s.assertUpdatedSecret(c, md.URL, newData, durationPtr(2*time.Hour), &newDescription, &newStatus, &newTags, 2)
}

func (s *SecretsSuite) TestUpdateRotateInterval(c *gc.C) {
	URL := secrets.NewSimpleURL("app/mariadb/password")
	URL.ControllerUUID = s.State.ControllerUUID()
	URL.ModelUUID = s.State.ModelUUID()
	cp := state.CreateSecretParams{
		Version:        1,
		ProviderLabel:  "juju",
		Type:           "blob",
		Path:           "app/mariadb/password",
		RotateInterval: time.Hour,
		Params:         nil,
		Data:           map[string]string{"foo": "bar"},
	}
	md, err := s.store.CreateSecret(URL, cp)
	c.Assert(err, jc.ErrorIsNil)
	s.assertUpdatedSecret(c, md.URL, nil, durationPtr(2*time.Hour), nil, nil, nil, 1)
}

func (s *SecretsSuite) TestUpdateData(c *gc.C) {
	URL := secrets.NewSimpleURL("app/mariadb/password")
	URL.ControllerUUID = s.State.ControllerUUID()
	URL.ModelUUID = s.State.ModelUUID()
	cp := state.CreateSecretParams{
		ProviderLabel:  "juju",
		Version:        1,
		Type:           "blob",
		Path:           "app/mariadb/password",
		RotateInterval: time.Hour,
		Description:    "my secret",
		Status:         secrets.StatusActive,
		Tags:           map[string]string{"hello": "world"},
		Params:         nil,
		Data:           map[string]string{"foo": "bar"},
	}
	md, err := s.store.CreateSecret(URL, cp)
	c.Assert(err, jc.ErrorIsNil)
	newData := map[string]string{"foo": "bar", "hello": "world"}
	s.assertUpdatedSecret(c, md.URL, newData, nil, nil, nil, nil, 2)
}

func (s *SecretsSuite) assertUpdatedSecret(c *gc.C, URL *secrets.URL, data map[string]string, rotateInterval *time.Duration, description *string, status *secrets.SecretStatus, tags *map[string]string, expectedRevision int) {
	created := s.Clock.Now().Round(time.Second).UTC()

	up := state.UpdateSecretParams{
		RotateInterval: rotateInterval,
		Description:    description,
		Status:         status,
		Tags:           tags,
		Params:         nil,
		Data:           data,
	}
	s.Clock.Advance(time.Hour)
	updated := s.Clock.Now().Round(time.Second).UTC()
	md, err := s.store.UpdateSecret(URL.WithRevision(0), up)
	c.Assert(err, jc.ErrorIsNil)
	expected := &secrets.SecretMetadata{
		URL:            md.URL,
		Path:           "app/mariadb/password",
		Version:        1,
		RotateInterval: md.RotateInterval,
		Status:         md.Status,
		Description:    md.Description,
		Tags:           md.Tags,
		ID:             1,
		Provider:       "juju",
		ProviderID:     "",
		Revision:       expectedRevision,
		CreateTime:     created,
		UpdateTime:     updated,
	}
	if rotateInterval != nil {
		expected.RotateInterval = *rotateInterval
	}
	if status != nil {
		expected.Status = *status
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
	val, err := s.store.GetSecretValue(md.URL.WithRevision(expectedRevision))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(val.EncodedValues(), jc.DeepEquals, expectedData)
}

func (s *SecretsSuite) TestUpdateConcurrent(c *gc.C) {
	URL := secrets.NewSimpleURL("app/mariadb/password")
	URL.ControllerUUID = s.State.ControllerUUID()
	URL.ModelUUID = s.State.ModelUUID()

	cp := state.CreateSecretParams{
		Version:        1,
		ProviderLabel:  "juju",
		Type:           "blob",
		Path:           "app/mariadb/password",
		RotateInterval: time.Hour,
		Params:         nil,
		Data:           map[string]string{"foo": "bar"},
	}
	md, err := s.store.CreateSecret(URL, cp)

	state.SetBeforeHooks(c, s.State, func() {
		up := state.UpdateSecretParams{
			RotateInterval: durationPtr(3 * time.Hour),
			Params:         nil,
			Data:           map[string]string{"foo": "baz", "goodbye": "world"},
		}
		md, err = s.store.UpdateSecret(md.URL.WithRevision(0), up)
		c.Assert(err, jc.ErrorIsNil)
	})
	newData := map[string]string{"foo": "bar", "hello": "world"}
	s.assertUpdatedSecret(c, md.URL, newData, durationPtr(2*time.Hour), nil, nil, nil, 3)
}

func (s *SecretsSuite) TestSecretRotated(c *gc.C) {
	URL := secrets.NewSimpleURL("app/mariadb/password")
	URL.ControllerUUID = s.State.ControllerUUID()
	URL.ModelUUID = s.State.ModelUUID()
	cp := state.CreateSecretParams{
		Version:        1,
		ProviderLabel:  "juju",
		Type:           "blob",
		Path:           "app/mariadb/password",
		RotateInterval: time.Hour,
		Params:         nil,
		Data:           map[string]string{"foo": "bar"},
	}
	md, err := s.store.CreateSecret(URL, cp)
	c.Assert(err, jc.ErrorIsNil)
	now := time.Now()
	err = s.State.SecretRotated(URL, now)
	c.Assert(err, jc.ErrorIsNil)

	rotated := state.GetSecretRotateTime(c, s.State, md.ID)
	c.Assert(rotated, gc.Equals, now.Round(time.Second))
}

func (s *SecretsSuite) TestSecretRotatedConcurrent(c *gc.C) {
	URL := secrets.NewSimpleURL("app/mariadb/password")
	URL.ControllerUUID = s.State.ControllerUUID()
	URL.ModelUUID = s.State.ModelUUID()
	cp := state.CreateSecretParams{
		Version:        1,
		ProviderLabel:  "juju",
		Type:           "blob",
		Path:           "app/mariadb/password",
		RotateInterval: time.Hour,
		Params:         nil,
		Data:           map[string]string{"foo": "bar"},
	}
	md, err := s.store.CreateSecret(URL, cp)
	c.Assert(err, jc.ErrorIsNil)

	now := time.Now()
	later := now.Add(time.Hour)
	state.SetBeforeHooks(c, s.State, func() {
		err := s.State.SecretRotated(URL, later)
		c.Assert(err, jc.ErrorIsNil)
	})

	err = s.State.SecretRotated(URL, now)
	c.Assert(err, jc.ErrorIsNil)

	rotated := state.GetSecretRotateTime(c, s.State, md.ID)
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

func (s *SecretsWatcherSuite) setupWatcher(c *gc.C) state.SecretsRotationWatcher {
	URL := secrets.NewSimpleURL("app/mariadb/password")
	md, err := s.store.CreateSecret(URL, state.CreateSecretParams{
		Version:        1,
		Owner:          "application-mariadb",
		Path:           "app/mariadb/password",
		RotateInterval: time.Hour,
	})
	c.Assert(err, jc.ErrorIsNil)
	w := s.State.WatchSecretsRotationChanges("application-mariadb")

	now := s.Clock.Now().Round(time.Second).UTC()
	wc := testing.NewSecretsRotationWatcherC(c, s.State, w)
	wc.AssertChange(watcher.SecretRotationChange{
		ID:             md.ID,
		URL:            md.URL,
		RotateInterval: time.Hour,
		LastRotateTime: now,
	})
	wc.AssertNoChange()
	return w
}

func (s *SecretsWatcherSuite) TestWatchInitialEvent(c *gc.C) {
	w := s.setupWatcher(c)
	testing.AssertStop(c, w)
}

func (s *SecretsWatcherSuite) TestWatchSingleUpdate(c *gc.C) {
	w := s.setupWatcher(c)
	wc := testing.NewSecretsRotationWatcherC(c, s.State, w)
	defer testing.AssertStop(c, w)

	URL := secrets.NewSimpleURL("app/mariadb/password")
	md, err := s.store.UpdateSecret(URL, state.UpdateSecretParams{
		RotateInterval: durationPtr(time.Minute),
	})
	c.Assert(err, jc.ErrorIsNil)

	wc.AssertChange(watcher.SecretRotationChange{
		ID:             md.ID,
		URL:            md.URL,
		RotateInterval: time.Minute,
		LastRotateTime: md.CreateTime.UTC(),
	})
	wc.AssertNoChange()
}

func (s *SecretsWatcherSuite) TestWatchDelete(c *gc.C) {
	w := s.setupWatcher(c)
	wc := testing.NewSecretsRotationWatcherC(c, s.State, w)
	defer testing.AssertStop(c, w)

	URL := secrets.NewSimpleURL("app/mariadb/password")
	md, err := s.store.UpdateSecret(URL, state.UpdateSecretParams{
		RotateInterval: durationPtr(0),
	})
	c.Assert(err, jc.ErrorIsNil)

	wc.AssertChange(watcher.SecretRotationChange{
		ID:             md.ID,
		URL:            md.URL,
		RotateInterval: 0,
	})
	wc.AssertNoChange()
}

func (s *SecretsWatcherSuite) TestWatchMultipleUpdatesSameSecret(c *gc.C) {
	w := s.setupWatcher(c)
	wc := testing.NewSecretsRotationWatcherC(c, s.State, w)
	defer testing.AssertStop(c, w)

	URL := secrets.NewSimpleURL("app/mariadb/password")
	_, err := s.store.UpdateSecret(URL, state.UpdateSecretParams{
		RotateInterval: durationPtr(time.Minute),
	})
	c.Assert(err, jc.ErrorIsNil)
	md, err := s.store.UpdateSecret(URL, state.UpdateSecretParams{
		RotateInterval: durationPtr(time.Second),
	})
	c.Assert(err, jc.ErrorIsNil)

	wc.AssertChange(watcher.SecretRotationChange{
		ID:             md.ID,
		URL:            md.URL,
		RotateInterval: time.Second,
		LastRotateTime: md.CreateTime.UTC(),
	})
	wc.AssertNoChange()
}

func (s *SecretsWatcherSuite) TestWatchMultipleUpdatesSameSecretDeleted(c *gc.C) {
	w := s.setupWatcher(c)
	wc := testing.NewSecretsRotationWatcherC(c, s.State, w)
	defer testing.AssertStop(c, w)

	URL := secrets.NewSimpleURL("app/mariadb/password")
	_, err := s.store.UpdateSecret(URL, state.UpdateSecretParams{
		RotateInterval: durationPtr(time.Minute),
	})
	c.Assert(err, jc.ErrorIsNil)
	md, err := s.store.UpdateSecret(URL, state.UpdateSecretParams{
		RotateInterval: durationPtr(0),
	})
	c.Assert(err, jc.ErrorIsNil)

	wc.AssertChange(watcher.SecretRotationChange{
		ID:             md.ID,
		URL:            md.URL,
		RotateInterval: 0,
	})
	wc.AssertNoChange()
}

func (s *SecretsWatcherSuite) TestWatchMultipleUpdates(c *gc.C) {
	w := s.setupWatcher(c)
	wc := testing.NewSecretsRotationWatcherC(c, s.State, w)
	defer testing.AssertStop(c, w)

	URL := secrets.NewSimpleURL("app/mariadb/password")
	_, err := s.store.UpdateSecret(URL, state.UpdateSecretParams{
		RotateInterval: durationPtr(time.Minute),
	})
	c.Assert(err, jc.ErrorIsNil)

	URL2 := secrets.NewSimpleURL("app/mariadb/password2")
	md2, err := s.store.CreateSecret(URL2, state.CreateSecretParams{
		Version:        1,
		Owner:          "application-mariadb",
		Path:           "app/mariadb/password2",
		RotateInterval: time.Hour,
	})
	c.Assert(err, jc.ErrorIsNil)

	md, err := s.store.UpdateSecret(URL, state.UpdateSecretParams{
		RotateInterval: durationPtr(0),
	})
	c.Assert(err, jc.ErrorIsNil)

	wc.AssertChange(watcher.SecretRotationChange{
		ID:             md2.ID,
		URL:            md2.URL,
		RotateInterval: time.Hour,
		LastRotateTime: md2.CreateTime.UTC(),
	}, watcher.SecretRotationChange{
		ID:             md.ID,
		URL:            md.URL,
		RotateInterval: 0,
	})
	wc.AssertNoChange()
}

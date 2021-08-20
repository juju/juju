// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/state"
)

type SecretsSuite struct {
	ConnSuite
	store state.SecretsStore
}

var _ = gc.Suite(&SecretsSuite{})

func (s *SecretsSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	s.store = state.NewSecretsStore(s.State)
}

func (s *SecretsSuite) TestCreate(c *gc.C) {
	p := state.CreateSecretParams{
		ControllerUUID: s.State.ControllerUUID(),
		ModelUUID:      s.State.ModelUUID(),
		Version:        1,
		Type:           "blob",
		Path:           "app.password",
		RotateDuration: time.Hour,
		Params:         nil,
		Data:           map[string]string{"foo": "bar"},
	}
	md, err := s.store.CreateSecret(p)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(md.URL.Revision, gc.Equals, 1)
	expectedURL := fmt.Sprintf("secret://v1/%s/%s/app.password", p.ControllerUUID, p.ModelUUID)
	c.Assert(md.URL.String(), gc.Equals, expectedURL)
	md.URL = nil
	now := s.Clock.Now().Round(time.Second).UTC()
	c.Assert(md, jc.DeepEquals, &secrets.SecretMetadata{
		Path:           p.Path,
		RotateDuration: time.Hour,
		Version:        1,
		Description:    "",
		Tags:           nil,
		ID:             1,
		ProviderID:     "",
		Revision:       1,
		CreateTime:     now,
		UpdateTime:     now,
	})

	_, err = s.store.CreateSecret(p)
	c.Assert(err, jc.Satisfies, errors.IsAlreadyExists)
}

func (s *SecretsSuite) TestCreateIncrementsID(c *gc.C) {
	p := state.CreateSecretParams{
		ControllerUUID: s.State.ControllerUUID(),
		ModelUUID:      s.State.ModelUUID(),
		Version:        1,
		Type:           "blob",
		Path:           "app.password",
		RotateDuration: time.Hour,
		Params:         nil,
		Data:           map[string]string{"foo": "bar"},
	}
	_, err := s.store.CreateSecret(p)
	c.Assert(err, jc.ErrorIsNil)

	p.Path = "app.password2"
	expectedURL := fmt.Sprintf("secret://v1/%s/%s/app.password2", p.ControllerUUID, p.ModelUUID)
	md, err := s.store.CreateSecret(p)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(md.URL.String(), gc.Equals, expectedURL)
	c.Assert(md.ID, gc.Equals, 2)
}

func (s *SecretsSuite) TestGetValueNotFound(c *gc.C) {
	URL, _ := secrets.ParseURL("secret://v1/app.password")
	_, err := s.store.GetSecretValue(URL)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *SecretsSuite) TestGetValue(c *gc.C) {
	p := state.CreateSecretParams{
		ControllerUUID: s.State.ControllerUUID(),
		ModelUUID:      s.State.ModelUUID(),
		Version:        1,
		ProviderLabel:  "juju",
		Type:           "blob",
		Path:           "app.password",
		RotateDuration: time.Hour,
		Params:         nil,
		Data:           map[string]string{"foo": "bar"},
	}
	md, err := s.store.CreateSecret(p)
	c.Assert(err, jc.ErrorIsNil)

	val, err := s.store.GetSecretValue(md.URL)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(val.EncodedValues(), jc.DeepEquals, map[string]string{
		"foo": "bar",
	})
}

func (s *SecretsSuite) TestGetValueAttribute(c *gc.C) {
	p := state.CreateSecretParams{
		ControllerUUID: s.State.ControllerUUID(),
		ModelUUID:      s.State.ModelUUID(),
		Version:        1,
		ProviderLabel:  "juju",
		Type:           "blob",
		Path:           "app.password",
		RotateDuration: time.Hour,
		Params:         nil,
		Data:           map[string]string{"foo": "bar", "hello": "world"},
	}
	md, err := s.store.CreateSecret(p)
	c.Assert(err, jc.ErrorIsNil)

	md.URL.Attribute = "hello"
	val, err := s.store.GetSecretValue(md.URL)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(val.EncodedValues(), jc.DeepEquals, map[string]string{
		"hello": "world",
	})
}

func (s *SecretsSuite) TestGetValueAttributeNotFound(c *gc.C) {
	p := state.CreateSecretParams{
		ControllerUUID: s.State.ControllerUUID(),
		ModelUUID:      s.State.ModelUUID(),
		Version:        1,
		ProviderLabel:  "juju",
		Type:           "blob",
		Path:           "app.password",
		RotateDuration: time.Hour,
		Params:         nil,
		Data:           map[string]string{"foo": "bar", "hello": "world"},
	}
	md, err := s.store.CreateSecret(p)
	c.Assert(err, jc.ErrorIsNil)

	md.URL.Attribute = "goodbye"
	_, err = s.store.GetSecretValue(md.URL)
	c.Assert(err, gc.ErrorMatches, `secret attribute "goodbye" not found`)
}

func (s *SecretsSuite) TestList(c *gc.C) {
	p := state.CreateSecretParams{
		ControllerUUID: s.State.ControllerUUID(),
		ModelUUID:      s.State.ModelUUID(),
		Version:        1,
		ProviderLabel:  "juju",
		Type:           "blob",
		Path:           "app.password",
		RotateDuration: time.Hour,
		Params:         nil,
		Data:           map[string]string{"foo": "bar"},
	}
	_, err := s.store.CreateSecret(p)
	c.Assert(err, jc.ErrorIsNil)

	list, err := s.store.ListSecrets(state.SecretsFilter{})
	c.Assert(err, jc.ErrorIsNil)
	now := s.Clock.Now().Round(time.Second).UTC()
	c.Assert(list, jc.DeepEquals, []*secrets.SecretMetadata{{
		Path:           "app.password",
		RotateDuration: time.Hour,
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

func (s *SecretsSuite) TestUpdate(c *gc.C) {
	cp := state.CreateSecretParams{
		ControllerUUID: s.State.ControllerUUID(),
		ModelUUID:      s.State.ModelUUID(),
		Version:        1,
		ProviderLabel:  "juju",
		Type:           "blob",
		Path:           "app.password",
		RotateDuration: time.Hour,
		Params:         nil,
		Data:           map[string]string{"foo": "bar"},
	}
	md, err := s.store.CreateSecret(cp)
	c.Assert(err, jc.ErrorIsNil)
	s.assertUpdatedSecret(c, md.URL, 2)
}

func (s *SecretsSuite) assertUpdatedSecret(c *gc.C, URL *secrets.URL, revision int) {
	created := s.Clock.Now().Round(time.Second).UTC()

	up := state.UpdateSecretParams{
		RotateDuration: 2 * time.Hour,
		Params:         nil,
		Data:           map[string]string{"foo": "bar", "hello": "world"},
	}
	s.Clock.Advance(time.Hour)
	updated := s.Clock.Now().Round(time.Second).UTC()
	md, err := s.store.UpdateSecret(URL.WithRevision(0), up)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(md.URL.Revision, gc.Equals, revision)
	c.Assert(md, jc.DeepEquals, &secrets.SecretMetadata{
		URL:            md.URL,
		Path:           "app.password",
		Version:        1,
		Description:    "",
		Tags:           map[string]string{},
		RotateDuration: 2 * time.Hour,
		ID:             1,
		Provider:       "juju",
		ProviderID:     "",
		Revision:       revision,
		CreateTime:     created,
		UpdateTime:     updated,
	})

	list, err := s.store.ListSecrets(state.SecretsFilter{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(list, jc.DeepEquals, []*secrets.SecretMetadata{{
		Path:           "app.password",
		RotateDuration: 2 * time.Hour,
		Version:        1,
		Description:    "",
		Tags:           map[string]string{},
		ID:             1,
		Provider:       "juju",
		ProviderID:     "",
		Revision:       revision,
		CreateTime:     created,
		UpdateTime:     updated,
	}})
}

func (s *SecretsSuite) TestUpdateConcurrent(c *gc.C) {
	cp := state.CreateSecretParams{
		ControllerUUID: s.State.ControllerUUID(),
		ModelUUID:      s.State.ModelUUID(),
		Version:        1,
		ProviderLabel:  "juju",
		Type:           "blob",
		Path:           "app.password",
		RotateDuration: time.Hour,
		Params:         nil,
		Data:           map[string]string{"foo": "bar"},
	}
	md, err := s.store.CreateSecret(cp)

	state.SetBeforeHooks(c, s.State, func() {
		up := state.UpdateSecretParams{
			RotateDuration: 3 * time.Hour,
			Params:         nil,
			Data:           map[string]string{"foo": "baz", "goodbye": "world"},
		}
		md, err = s.store.UpdateSecret(md.URL.WithRevision(0), up)
		c.Assert(err, jc.ErrorIsNil)
	})
	s.assertUpdatedSecret(c, md.URL, 3)
}

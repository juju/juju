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
		Scope:          "application",
		Params:         nil,
		Data:           map[string]string{"foo": "bar"},
	}
	URL, md, err := s.store.CreateSecret(p)
	c.Assert(err, jc.ErrorIsNil)
	expectedURL := fmt.Sprintf("secret://v1/%s/%s/app.password", p.ControllerUUID, p.ModelUUID)
	c.Assert(URL.String(), gc.Equals, expectedURL)
	now := s.Clock.Now().Round(time.Second).UTC()
	c.Assert(md, jc.DeepEquals, &secrets.SecretMetadata{
		Path:        p.Path,
		Scope:       secrets.Scope(p.Scope),
		Version:     1,
		Description: "",
		Tags:        nil,
		ID:          1,
		ProviderID:  "",
		Revision:    1,
		CreateTime:  now,
		UpdateTime:  now,
	})

	_, _, err = s.store.CreateSecret(p)
	c.Assert(err, jc.Satisfies, errors.IsAlreadyExists)
}

func (s *SecretsSuite) TestCreateIncrementsID(c *gc.C) {
	p := state.CreateSecretParams{
		ControllerUUID: s.State.ControllerUUID(),
		ModelUUID:      s.State.ModelUUID(),
		Version:        1,
		Type:           "blob",
		Path:           "app.password",
		Scope:          "application",
		Params:         nil,
		Data:           map[string]string{"foo": "bar"},
	}
	_, _, err := s.store.CreateSecret(p)
	c.Assert(err, jc.ErrorIsNil)

	p.Path = "app.password2"
	expectedURL := fmt.Sprintf("secret://v1/%s/%s/app.password2", p.ControllerUUID, p.ModelUUID)
	URL, md, err := s.store.CreateSecret(p)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(URL.String(), gc.Equals, expectedURL)
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
		Type:           "blob",
		Path:           "app.password",
		Scope:          "application",
		Params:         nil,
		Data:           map[string]string{"foo": "bar"},
	}
	URL, _, err := s.store.CreateSecret(p)
	c.Assert(err, jc.ErrorIsNil)

	val, err := s.store.GetSecretValue(URL)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(val.EncodedValues(), jc.DeepEquals, map[string]string{
		"foo": "bar",
	})
}

func (s *SecretsSuite) TestList(c *gc.C) {
	p := state.CreateSecretParams{
		ControllerUUID: s.State.ControllerUUID(),
		ModelUUID:      s.State.ModelUUID(),
		Version:        1,
		Type:           "blob",
		Path:           "app.password",
		Scope:          "application",
		Params:         nil,
		Data:           map[string]string{"foo": "bar"},
	}
	_, _, err := s.store.CreateSecret(p)
	c.Assert(err, jc.ErrorIsNil)

	list, err := s.store.ListSecrets(state.SecretsFilter{})
	c.Assert(err, jc.ErrorIsNil)
	now := s.Clock.Now().Round(time.Second).UTC()
	c.Assert(list, jc.DeepEquals, []*secrets.SecretMetadata{{
		Path:        "app.password",
		Scope:       "application",
		Version:     1,
		Description: "",
		Tags:        map[string]string{},
		ID:          1,
		ProviderID:  "",
		Revision:    1,
		CreateTime:  now,
		UpdateTime:  now,
	}})
}

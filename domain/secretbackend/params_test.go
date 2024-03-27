// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackend

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	backenderrors "github.com/juju/juju/domain/secretbackend/errors"
)

type paramsSuite struct{}

var _ = gc.Suite(&paramsSuite{})

func (s *paramsSuite) TestCreateSecretBackendParamsValidate(c *gc.C) {
	p := CreateSecretBackendParams{}
	err := p.Validate()
	c.Check(err, jc.ErrorIs, backenderrors.NotValid)
	c.Check(err, gc.ErrorMatches, `secret backend not valid: ID is missing`)

	p = CreateSecretBackendParams{
		ID: "backend-id",
	}
	err = p.Validate()
	c.Check(err, jc.ErrorIs, backenderrors.NotValid)
	c.Check(err, gc.ErrorMatches, `secret backend not valid: name is missing`)

	p = CreateSecretBackendParams{
		ID:   "backend-id",
		Name: "backend-name",
	}
	err = p.Validate()
	c.Check(err, jc.ErrorIs, backenderrors.NotValid)
	c.Check(err, gc.ErrorMatches, `secret backend not valid: type is missing`)

	p = CreateSecretBackendParams{
		ID:          "backend-id",
		Name:        "backend-name",
		BackendType: "vault",
		Config: map[string]string{
			"": "value",
		},
	}
	err = p.Validate()
	c.Check(err, jc.ErrorIs, backenderrors.NotValid)
	c.Check(err, gc.ErrorMatches, `secret backend not valid: empty config key for "backend-name"`)

	p = CreateSecretBackendParams{
		ID:          "backend-id",
		Name:        "backend-name",
		BackendType: "vault",
		Config: map[string]string{
			"key": "",
		},
	}
	err = p.Validate()
	c.Check(err, jc.ErrorIs, backenderrors.NotValid)
	c.Check(err, gc.ErrorMatches, `secret backend not valid: empty config value for "backend-name"`)
}

func (s *paramsSuite) TestUpdateSecretBackendParamsValidate(c *gc.C) {
	p := UpdateSecretBackendParams{}
	err := p.Validate()
	c.Check(err, jc.ErrorIs, backenderrors.NotValid)
	c.Check(err, gc.ErrorMatches, `secret backend not valid: both ID and name are missing`)

	p = UpdateSecretBackendParams{
		ID:   "backend-id",
		Name: "backend-name",
	}
	err = p.Validate()
	c.Check(err, jc.ErrorIs, backenderrors.NotValid)
	c.Check(err, gc.ErrorMatches, `secret backend not valid: both ID and name are set`)

	p = UpdateSecretBackendParams{
		ID:      "backend-id",
		NewName: ptr(""),
	}
	err = p.Validate()
	c.Check(err, jc.ErrorIs, backenderrors.NotValid)
	c.Check(err, gc.ErrorMatches, `secret backend not valid: name cannot be set to empty`)

	p = UpdateSecretBackendParams{
		ID: "backend-id",
		Config: map[string]string{
			"": "value",
		},
	}
	err = p.Validate()
	c.Check(err, jc.ErrorIs, backenderrors.NotValid)
	c.Check(err, gc.ErrorMatches, `secret backend not valid: empty config key for "backend-id"`)

	p = UpdateSecretBackendParams{
		ID: "backend-id",
		Config: map[string]string{
			"key": "",
		},
	}
	err = p.Validate()
	c.Check(err, jc.ErrorIs, backenderrors.NotValid)
	c.Check(err, gc.ErrorMatches, `secret backend not valid: empty config value for "backend-id"`)
}

func ptr[T any](s T) *T {
	return &s
}

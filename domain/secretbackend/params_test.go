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

func (s *paramsSuite) TestUpsertSecretBackendParamsValidate(c *gc.C) {
	p := UpsertSecretBackendParams{}
	err := p.Validate()
	c.Check(err, jc.ErrorIs, backenderrors.NotValid)
	c.Check(err, gc.ErrorMatches, `secret backend not valid: ID is missing`)

	p = UpsertSecretBackendParams{
		ID: "backend-id",
		Config: map[string]interface{}{
			"": "value",
		},
	}
	err = p.Validate()
	c.Check(err, jc.ErrorIs, backenderrors.NotValid)
	c.Check(err, gc.ErrorMatches, `secret backend not valid: empty config key for "backend-id"`)

	p = UpsertSecretBackendParams{
		ID: "backend-id",
		Config: map[string]interface{}{
			"key": "",
		},
	}
	err = p.Validate()
	c.Check(err, jc.ErrorIs, backenderrors.NotValid)
	c.Check(err, gc.ErrorMatches, `secret backend not valid: empty config value for "backend-id"`)
}

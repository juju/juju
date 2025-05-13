// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackend

import (
	"github.com/juju/tc"

	backenderrors "github.com/juju/juju/domain/secretbackend/errors"
)

type paramsSuite struct{}

var _ = tc.Suite(&paramsSuite{})

func (s *paramsSuite) TestBackendIdentifierString(c *tc.C) {
	id := BackendIdentifier{
		ID:   "backend-id",
		Name: "backend-name",
	}
	c.Check(id.String(), tc.Equals, "backend-name")

	id.Name = ""
	c.Check(id.String(), tc.Equals, "backend-id")
}

func (s *paramsSuite) TestCreateSecretBackendParamsValidate(c *tc.C) {
	p := CreateSecretBackendParams{}
	err := p.Validate()
	c.Check(err, tc.ErrorIs, backenderrors.NotValid)
	c.Check(err, tc.ErrorMatches, `secret backend not valid: ID is missing`)

	p = CreateSecretBackendParams{
		BackendIdentifier: BackendIdentifier{
			ID: "backend-id",
		},
	}
	err = p.Validate()
	c.Check(err, tc.ErrorIs, backenderrors.NotValid)
	c.Check(err, tc.ErrorMatches, `secret backend not valid: name is missing`)

	p = CreateSecretBackendParams{
		BackendIdentifier: BackendIdentifier{
			ID:   "backend-id",
			Name: "backend-name",
		},
	}
	err = p.Validate()
	c.Check(err, tc.ErrorIs, backenderrors.NotValid)
	c.Check(err, tc.ErrorMatches, `secret backend not valid: type is missing`)

	p = CreateSecretBackendParams{
		BackendIdentifier: BackendIdentifier{
			ID:   "backend-id",
			Name: "backend-name",
		},
		BackendType: "vault",
		Config: map[string]string{
			"": "value",
		},
	}
	err = p.Validate()
	c.Check(err, tc.ErrorIs, backenderrors.NotValid)
	c.Check(err, tc.ErrorMatches, `secret backend not valid: empty config key for "backend-name"`)

	p = CreateSecretBackendParams{
		BackendIdentifier: BackendIdentifier{
			ID:   "backend-id",
			Name: "backend-name",
		},
		BackendType: "vault",
		Config: map[string]string{
			"key": "",
		},
	}
	err = p.Validate()
	c.Check(err, tc.ErrorIs, backenderrors.NotValid)
	c.Check(err, tc.ErrorMatches, `secret backend not valid: empty config value for "backend-name"`)
}

func (s *paramsSuite) TestUpdateSecretBackendParamsValidate(c *tc.C) {
	p := UpdateSecretBackendParams{}
	err := p.Validate()
	c.Check(err, tc.ErrorIs, backenderrors.NotValid)
	c.Check(err, tc.ErrorMatches, `secret backend not valid: both ID and name are missing`)

	p = UpdateSecretBackendParams{
		BackendIdentifier: BackendIdentifier{
			ID:   "backend-id",
			Name: "backend-name",
		},
	}
	err = p.Validate()
	c.Check(err, tc.ErrorIs, backenderrors.NotValid)
	c.Check(err, tc.ErrorMatches, `secret backend not valid: both ID and name are set`)

	p = UpdateSecretBackendParams{
		BackendIdentifier: BackendIdentifier{
			ID: "backend-id",
		},
		NewName: ptr(""),
	}
	err = p.Validate()
	c.Check(err, tc.ErrorIs, backenderrors.NotValid)
	c.Check(err, tc.ErrorMatches, `secret backend not valid: name cannot be set to empty`)

	p = UpdateSecretBackendParams{
		BackendIdentifier: BackendIdentifier{
			ID: "backend-id",
		},
		Config: map[string]string{
			"": "value",
		},
	}
	err = p.Validate()
	c.Check(err, tc.ErrorIs, backenderrors.NotValid)
	c.Check(err, tc.ErrorMatches, `secret backend not valid: empty config key for "backend-id"`)

	p = UpdateSecretBackendParams{
		BackendIdentifier: BackendIdentifier{
			ID: "backend-id",
		},
		Config: map[string]string{
			"key": "",
		},
	}
	err = p.Validate()
	c.Check(err, tc.ErrorIs, backenderrors.NotValid)
	c.Check(err, tc.ErrorMatches, `secret backend not valid: empty config value for "backend-id"`)
}

func ptr[T any](s T) *T {
	return &s
}

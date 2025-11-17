// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager

import (
	"testing"

	"github.com/juju/tc"

	corecredential "github.com/juju/juju/core/credential"
	coreerrors "github.com/juju/juju/core/errors"
	usertesting "github.com/juju/juju/core/user/testing"
)

// createTypesSuite tests the types defined by the model manager for creating
// new models. This suite is used to test the various validity states.
type createTypesSuite struct{}

// TestCreateTypesSuite runs all the tests in the [createTypesSuite].
func TestCreateTypesSuite(t *testing.T) {
	tc.Run(t, &createTypesSuite{})
}

// TestCreationArgsWithoutCredential is testing that a [CreationArgs] struct
// passes validation when there is no credential set. It is expected that some
// models are created without cloud credentials. This is a happy path test.
func (*createTypesSuite) TestCreationArgsWithoutCredential(c *tc.C) {
	args := CreationArgs{
		Cloud:         "my-cloud",
		CloudRegion:   "my-region",
		Name:          "my-awesome-model",
		Owner:         usertesting.GenUserUUID(c),
		SecretBackend: "foobar",
	}

	c.Check(args.Validate(), tc.ErrorIsNil)
}

// TestCreationArgsWithoutSecretBackend is testing that a [CreationArgs] struct
// passes validation when there is no secret backend set. It is expected that
// some models are created without a secret backend defined by the caller. This
// is a happy path test.
func (*createTypesSuite) TestCreationArgsWithoutSecretBackend(c *tc.C) {
	args := CreationArgs{
		Cloud:       "my-cloud",
		CloudRegion: "my-region",
		Credential: corecredential.Key{
			Owner: usertesting.GenNewName(c, "tlm"),
			Cloud: "my-cloud",
			Name:  "my-credential",
		},
		Name:  "my-awesome-model",
		Owner: usertesting.GenUserUUID(c),
	}

	c.Check(args.Validate(), tc.ErrorIsNil)
}

// TestCreationArgsWithoutCloudRegion is testing that a [CreationArgs] struct
// passes validation when there is no cloud region set. It is expected that
// some models are created without a cloud region defined by the caller. This
// is a happy path test.
func (*createTypesSuite) TestCreationArgsWithoutCloudRegion(c *tc.C) {
	args := CreationArgs{
		Cloud: "my-cloud",
		Credential: corecredential.Key{
			Owner: usertesting.GenNewName(c, "tlm"),
			Cloud: "my-cloud",
			Name:  "my-credential",
		},
		Name:          "my-awesome-model",
		Owner:         usertesting.GenUserUUID(c),
		SecretBackend: "mysecretbackend",
	}

	c.Check(args.Validate(), tc.ErrorIsNil)
}

// TestCreationArgsAllSet is testing that a [CreationArgs] struct passes
// validation when all the fields are set. This is a happy path test.
func (*createTypesSuite) TestCreationArgsAllSet(c *tc.C) {
	args := CreationArgs{
		Credential: corecredential.Key{
			Cloud: "my-cloud",
			Owner: usertesting.GenNewName(c, "tlm"),
			Name:  "my-credential",
		},
		Cloud:         "my-cloud",
		CloudRegion:   "my-region",
		Name:          "my-awesome-model",
		Owner:         usertesting.GenUserUUID(c),
		SecretBackend: "mysecretbackend",
	}

	c.Check(args.Validate(), tc.ErrorIsNil)
}

// TestCreationArgsValidationError is asserting all the cases that
// [CreationArgs] should fail validation for. We expect that when a
// [CreationArgs] struct is invalid the caller gets back an error that
// satisfies [coreerrors.NotValid].
func (*createTypesSuite) TestCreationArgsValidationError(c *tc.C) {
	tests := []struct {
		Args CreationArgs
		Name string
	}{
		// Name cannot be zero value.
		{
			Name: "Test invalid name",
			Args: CreationArgs{
				Cloud:       "my-cloud",
				CloudRegion: "my-region",
				Name:        "",
				Owner:       usertesting.GenUserUUID(c),
			},
		},
		// Owner uuid cannot be zero value.
		{
			Name: "Test invalid owner",
			Args: CreationArgs{
				Cloud:       "my-cloud",
				CloudRegion: "my-region",
				Name:        "my-awesome-model",
				Owner:       "",
			},
		},
		// Cloud cannot be a zero value
		{
			Name: "Test invalid cloud",
			Args: CreationArgs{
				Cloud:       "",
				CloudRegion: "my-region",
				Name:        "my-awesome-model",
				Owner:       usertesting.GenUserUUID(c),
			},
		},
		// Testing that credential is a valid credential key
		{
			Name: "Test invalid credential key",
			Args: CreationArgs{
				Cloud:       "my-cloud",
				CloudRegion: "my-region",
				Credential: corecredential.Key{
					Owner: usertesting.GenNewName(c, "wallyworld"),
				},
				Name:  "my-awesome-model",
				Owner: usertesting.GenUserUUID(c),
			},
		},
	}

	for _, test := range tests {
		c.Run(test.Name, func(t *testing.T) {
			tc.Check(t, test.Args.Validate(), tc.ErrorIs, coreerrors.NotValid)
		})
	}
}

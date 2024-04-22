// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/credential"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	coreuser "github.com/juju/juju/core/user"
	usertesting "github.com/juju/juju/core/user/testing"
)

type typesSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&typesSuite{})

func (*typesSuite) TestModelCreationArgsValidation(c *gc.C) {
	userUUID := usertesting.GenUserUUID(c)

	tests := []struct {
		Args    ModelCreationArgs
		ErrTest error
	}{
		{
			Args: ModelCreationArgs{
				Cloud:       "my-cloud",
				CloudRegion: "my-region",
				Name:        "",
				Owner:       userUUID,
			},
			ErrTest: errors.NotValid,
		},
		{
			Args: ModelCreationArgs{
				Cloud:       "my-cloud",
				CloudRegion: "my-region",
				Name:        "my-awesome-model",
				Owner:       "",
			},
			ErrTest: errors.NotValid,
		},
		{
			Args: ModelCreationArgs{
				Cloud:       "",
				CloudRegion: "my-region",
				Name:        "my-awesome-model",
				Owner:       userUUID,
			},
			ErrTest: errors.NotValid,
		},
		{
			Args: ModelCreationArgs{
				Cloud:       "my-cloud",
				CloudRegion: "",
				Name:        "my-awesome-model",
				Owner:       userUUID,
			},
			ErrTest: nil,
		},
		{
			Args: ModelCreationArgs{
				Cloud:       "my-cloud",
				CloudRegion: "my-region",
				Credential: credential.Key{
					Owner: "wallyworld",
				},
				Name:  "my-awesome-model",
				Owner: userUUID,
			},
			ErrTest: errors.NotValid,
		},
		{
			Args: ModelCreationArgs{
				Cloud:       "my-cloud",
				CloudRegion: "my-region",
				Name:        "my-awesome-model",
				Owner:       userUUID,
				UUID:        coremodel.UUID("not-valid"),
			},
			ErrTest: errors.NotValid,
		},
		{
			Args: ModelCreationArgs{
				Cloud:       "my-cloud",
				CloudRegion: "my-region",
				Name:        "my-awesome-model",
				Owner:       userUUID,
			},
			ErrTest: nil,
		},
		{
			Args: ModelCreationArgs{
				Cloud:       "my-cloud",
				CloudRegion: "my-region",
				Credential: credential.Key{
					Cloud: "cloud",
					Owner: "wallyworld",
					Name:  "mycred",
				},
				Name:  "my-awesome-model",
				Owner: userUUID,
			},
			ErrTest: nil,
		},
	}

	for i, test := range tests {
		c.Logf("testing: %d %v", i, test.Args)

		err := test.Args.Validate()
		if test.ErrTest == nil {
			c.Check(err, jc.ErrorIsNil)
		} else {
			c.Check(err, jc.ErrorIs, test.ErrTest)
		}
	}
}

func (*typesSuite) TestModelToReadOnlyModel(c *gc.C) {
	modelUUID := modeltesting.GenModelUUID(c)
	userUUID := usertesting.GenUserUUID(c)
	controllerUUID := modeltesting.GenModelUUID(c)

	args := ModelCreationArgs{
		UUID:        modelUUID,
		Cloud:       "my-cloud",
		CloudRegion: "my-region",
		Credential: credential.Key{
			Cloud: "cloud",
			Owner: "wallyworld",
			Name:  "mycred",
		},
		Name:  "my-awesome-model",
		Owner: userUUID,
	}

	model := args.AsReadOnly(controllerUUID, coremodel.IAAS, coreuser.AdminUserName)
	c.Check(model, gc.DeepEquals, ReadOnlyModelCreationArgs{
		UUID:            modelUUID,
		ControllerUUID:  controllerUUID,
		Name:            "my-awesome-model",
		Owner:           coreuser.AdminUserName,
		Type:            coremodel.IAAS,
		Cloud:           "my-cloud",
		CloudRegion:     "my-region",
		CredentialOwner: "wallyworld",
		CredentialName:  "mycred",
	})
}

func (*typesSuite) TestReadOnlyModelCreationArgsValidation(c *gc.C) {
	modelUUID := modeltesting.GenModelUUID(c)
	userUUID := usertesting.GenUserUUID(c)
	controllerUUID := modeltesting.GenModelUUID(c)

	tests := []struct {
		Args    ModelCreationArgs
		ErrTest error
	}{
		{
			Args: ModelCreationArgs{
				UUID:        modelUUID,
				Cloud:       "my-cloud",
				CloudRegion: "my-region",
				Name:        "",
				Owner:       userUUID,
			},
			ErrTest: errors.NotValid,
		},
		{
			Args: ModelCreationArgs{
				UUID:        modelUUID,
				Cloud:       "",
				CloudRegion: "my-region",
				Name:        "my-awesome-model",
				Owner:       userUUID,
			},
			ErrTest: errors.NotValid,
		},
		{
			Args: ModelCreationArgs{
				UUID:        modelUUID,
				Cloud:       "my-cloud",
				CloudRegion: "",
				Name:        "my-awesome-model",
				Owner:       userUUID,
			},
			ErrTest: nil,
		},
		{
			// This would be an invalid ModelCreationArgs validation, but
			// ReadOnlyModelCreationArgs does not validate the credential in
			// the same way.
			Args: ModelCreationArgs{
				UUID:        modelUUID,
				Cloud:       "my-cloud",
				CloudRegion: "my-region",
				Credential: credential.Key{
					Owner: "wallyworld",
				},
				Name:  "my-awesome-model",
				Owner: userUUID,
			},
			ErrTest: nil,
		},
		{
			Args: ModelCreationArgs{
				Cloud:       "my-cloud",
				CloudRegion: "my-region",
				Name:        "my-awesome-model",
				Owner:       userUUID,
				UUID:        coremodel.UUID("not-valid"),
			},
			ErrTest: errors.NotValid,
		},
		{
			Args: ModelCreationArgs{
				UUID:        modelUUID,
				Cloud:       "my-cloud",
				CloudRegion: "my-region",
				Name:        "my-awesome-model",
				Owner:       userUUID,
			},
			ErrTest: nil,
		},
		{
			Args: ModelCreationArgs{
				UUID:        modelUUID,
				Cloud:       "my-cloud",
				CloudRegion: "my-region",
				Credential: credential.Key{
					Cloud: "cloud",
					Owner: "wallyworld",
					Name:  "mycred",
				},
				Name:  "my-awesome-model",
				Owner: userUUID,
			},
			ErrTest: nil,
		},
	}

	for i, test := range tests {
		c.Logf("testing: %d %v", i, test.Args)

		err := test.Args.AsReadOnly(controllerUUID, coremodel.CAAS, coreuser.AdminUserName).Validate()
		if test.ErrTest == nil {
			c.Check(err, jc.ErrorIsNil)
		} else {
			c.Check(err, jc.ErrorIs, test.ErrTest)
		}
	}
}

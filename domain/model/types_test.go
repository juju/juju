// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/credential"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/user"
)

type typesSuite struct {
}

var _ = gc.Suite(&typesSuite{})

func (*typesSuite) TestModelCreationArgsValidation(c *gc.C) {
	userUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
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
				Cloud:       "my-cloud",
				CloudRegion: "my-region",
				Name:        "my-awesome-model",
				Owner:       userUUID,
			},
			ErrTest: errors.NotSupported,
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
				Credential: credential.ID{
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
				Credential: credential.ID{
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

	for _, test := range tests {
		err := test.Args.Validate()
		if test.ErrTest == nil {
			c.Assert(err, jc.ErrorIsNil)
		} else {
			c.Assert(err, jc.ErrorIs, test.ErrTest)
		}
	}
}

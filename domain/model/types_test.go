// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	gc "gopkg.in/check.v1"

	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/domain/credential"
)

type typesSuite struct {
}

var _ = gc.Suite(&typesSuite{})

func (*typesSuite) TestUUIDValidate(c *gc.C) {
	tests := []struct {
		uuid string
		err  *string
	}{
		{
			uuid: "",
			err:  ptr("empty uuid"),
		},
		{
			uuid: "invalid",
			err:  ptr("invalid uuid.*"),
		},
		{
			uuid: utils.MustNewUUID().String(),
		},
	}

	for i, test := range tests {
		c.Logf("test %d: %q", i, test.uuid)
		err := UUID(test.uuid).Validate()

		if test.err == nil {
			c.Check(err, gc.IsNil)
			continue
		}

		c.Check(err, gc.ErrorMatches, *test.err)
	}
}

func ptr[T any](v T) *T {
	return &v
}

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
				Type:        coremodel.CAAS,
			},
			ErrTest: errors.NotValid,
		},
		{
			Args: ModelCreationArgs{
				Cloud:       "my-cloud",
				CloudRegion: "my-region",
				Name:        "my-awesome-model",
				Owner:       "",
				Type:        coremodel.CAAS,
			},
			ErrTest: errors.NotValid,
		},
		{
			Args: ModelCreationArgs{
				Cloud:       "my-cloud",
				CloudRegion: "my-region",
				Name:        "my-awesome-model",
				Owner:       userUUID,
				Type:        coremodel.ModelType("ipv6-only"),
			},
			ErrTest: errors.NotSupported,
		},
		{
			Args: ModelCreationArgs{
				Cloud:       "",
				CloudRegion: "my-region",
				Name:        "my-awesome-model",
				Owner:       userUUID,
				Type:        coremodel.IAAS,
			},
			ErrTest: errors.NotValid,
		},
		{
			Args: ModelCreationArgs{
				Cloud:       "my-cloud",
				CloudRegion: "",
				Name:        "my-awesome-model",
				Owner:       userUUID,
				Type:        coremodel.IAAS,
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
				Type:  coremodel.IAAS,
			},
			ErrTest: errors.NotValid,
		},
		{
			Args: ModelCreationArgs{
				Cloud:       "my-cloud",
				CloudRegion: "my-region",
				Name:        "my-awesome-model",
				Owner:       userUUID,
				Type:        coremodel.IAAS,
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
				Type:  coremodel.IAAS,
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

// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/user"
	"github.com/juju/juju/domain/credential"
	jujuversion "github.com/juju/juju/version"
)

type typesSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&typesSuite{})

func (s *typesSuite) TestUUIDValidate(c *gc.C) {
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

func (s *typesSuite) TestModelCreationArgsValidation(c *gc.C) {
	userUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	tests := []struct {
		TestName string
		Args     ModelCreationArgs
		ErrTest  error
	}{
		{
			TestName: "test model creation args with zero value agent version fails",
			Args: ModelCreationArgs{
				Cloud:       "my-cloud",
				CloudRegion: "my-region",
				Name:        "",
				Owner:       userUUID,
				Type:        TypeCAAS,
			},
			ErrTest: errors.NotValid,
		},
		{
			TestName: "test model creation args with empty name fails",
			Args: ModelCreationArgs{
				AgentVersion: jujuversion.Current,
				Cloud:        "my-cloud",
				CloudRegion:  "my-region",
				Name:         "",
				Owner:        "wallyworld-ipv6",
				Type:         TypeCAAS,
			},
			ErrTest: errors.NotValid,
		},
		{
			TestName: "test model creation args with empty owner fails",
			Args: ModelCreationArgs{
				Cloud:       "my-cloud",
				CloudRegion: "my-region",
				Name:        "my-awesome-model",
				Owner:       userUUID,
				Type:        Type("ipv6-only"),
			},
			ErrTest: errors.NotSupported,
		},
		{
			TestName: "test model creation args with empty cloud fails",
			Args: ModelCreationArgs{
				Cloud:       "",
				CloudRegion: "my-region",
				Name:        "my-awesome-model",
				Owner:       userUUID,
				Type:        TypeIAAS,
			},
			ErrTest: errors.NotValid,
		},
		{
			TestName: "test model creation args with empty cloud region doesn't fail",
			Args: ModelCreationArgs{
				Cloud:       "my-cloud",
				CloudRegion: "",
				Name:        "my-awesome-model",
				Owner:       userUUID,
				Type:        TypeIAAS,
			},
			ErrTest: nil,
		},
		{
			TestName: "test model creation args with invalid credential fails",
			Args: ModelCreationArgs{
				AgentVersion: jujuversion.Current,
				Cloud:        "my-cloud",
				CloudRegion:  "my-region",
				Credential: credential.ID{
					Owner: "wallyworld",
				},
				Name:  "my-awesome-model",
				Owner: userUUID,
				Type:  TypeIAAS,
			},
			ErrTest: errors.NotValid,
		},
		{
			TestName: "test model creation args happy path 1",
			Args: ModelCreationArgs{
				Cloud:       "my-cloud",
				CloudRegion: "my-region",
				Name:        "my-awesome-model",
				Owner:       userUUID,
				Type:        TypeIAAS,
			},
			ErrTest: nil,
		},
		{
			TestName: "test model creation args happy path 2",
			Args: ModelCreationArgs{
				AgentVersion: jujuversion.Current,
				Cloud:        "my-cloud",
				CloudRegion:  "my-region",
				Credential: credential.ID{
					Cloud: "cloud",
					Owner: "wallyworld",
					Name:  "mycred",
				},
				Name:  "my-awesome-model",
				Owner: userUUID,
				Type:  TypeIAAS,
			},
			ErrTest: nil,
		},
	}

	for _, test := range tests {
		err := test.Args.Validate()
		if test.ErrTest == nil {
			c.Assert(err, jc.ErrorIsNil, gc.Commentf(test.TestName))
		} else {
			c.Assert(err, jc.ErrorIs, test.ErrTest, gc.Commentf(test.TestName))
		}
	}
}

func (s *typesSuite) TestValidModelTypes(c *gc.C) {
	validTypes := []Type{
		TypeCAAS,
		TypeIAAS,
	}

	for _, vt := range validTypes {
		c.Assert(vt.IsValid(), jc.IsTrue)
	}
}

func ptr[T any](v T) *T {
	return &v
}

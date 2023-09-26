// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	. "gopkg.in/check.v1"

	"github.com/juju/juju/domain/credential"
	"github.com/juju/juju/domain/model"
	modelerrors "github.com/juju/juju/domain/model/errors"
	modeltesting "github.com/juju/juju/domain/model/testing"
)

type dummyState struct {
	credentials      map[string]credential.ID
	modelCredentials map[string]credential.ID
}

type serviceSuite struct {
	testing.IsolationSuite

	modelUUID model.UUID
	state     *dummyState
}

var _ = Suite(&serviceSuite{})

func (d *dummyState) addCredential(cred credential.ID) {
	d.credentials[cred.String()] = cred
}

func (d *dummyState) SetCloudCredential(
	_ context.Context,
	uuid model.UUID,
	id credential.ID,
) error {
	cred, exists := d.credentials[id.String()]
	if !exists {
		return fmt.Errorf("%w credential %q", errors.NotFound, id)
	}

	if _, exists = d.modelCredentials[uuid.String()]; !exists {
		return fmt.Errorf("%w with uuid %q", modelerrors.NotFound, uuid)
	}

	d.modelCredentials[uuid.String()] = cred
	return nil
}

func (s *serviceSuite) SetUpTest(c *C) {
	s.modelUUID = modeltesting.GenModelUUID(c)
	s.state = &dummyState{
		credentials: map[string]credential.ID{},
		modelCredentials: map[string]credential.ID{
			s.modelUUID.String(): {},
		},
	}
}

func (s *serviceSuite) TestSetModelCredential(c *C) {
	cred := credential.ID{
		Cloud: "testcloud",
		Owner: "wallyworld",
		Name:  "ipv6world",
	}

	s.state.addCredential(cred)

	tag, err := names.ParseCloudCredentialTag("cloudcred-testcloud_wallyworld_ipv6world")
	c.Assert(err, jc.ErrorIsNil)

	svc := NewService(s.state)
	err = svc.SetCloudCredential(context.Background(), s.modelUUID, tag)
	c.Assert(err, jc.ErrorIsNil)

	foundCred, exists := s.state.modelCredentials[s.modelUUID.String()]
	c.Assert(exists, jc.IsTrue)
	c.Assert(foundCred, jc.DeepEquals, cred)
}

func (s *serviceSuite) TestSetModelCredentialModelNotFound(c *C) {
	cred := credential.ID{
		Cloud: "testcloud",
		Owner: "wallyworld",
		Name:  "ipv6world",
	}

	s.state.addCredential(cred)

	tag, err := names.ParseCloudCredentialTag("cloudcred-testcloud_wallyworld_ipv6world")
	c.Assert(err, jc.ErrorIsNil)

	svc := NewService(s.state)
	err = svc.SetCloudCredential(context.Background(), "noexist", tag)
	c.Assert(err, jc.ErrorIs, modelerrors.NotFound)
}

func (s *serviceSuite) TestSetModelCredentialNotFound(c *C) {
	tag, err := names.ParseCloudCredentialTag("cloudcred-testcloud_wallyworld_ipv6world")
	c.Assert(err, jc.ErrorIsNil)

	svc := NewService(s.state)
	err = svc.SetCloudCredential(context.Background(), s.modelUUID, tag)
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

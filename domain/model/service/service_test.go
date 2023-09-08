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
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/domain/modelmanager/service"
)

type dummyState struct {
	credentials      map[string]credential.ID
	modelCredentials map[string]credential.ID
}

type serviceSuite struct {
	testing.IsolationSuite

	state *dummyState
}

var _ = Suite(&serviceSuite{})

var (
	modelUUID = service.UUID("123")
)

func (d *dummyState) addCredential(cred credential.ID) {
	d.credentials[cred.String()] = cred
}

func (d *dummyState) SetCloudCredential(
	_ context.Context,
	uuid service.UUID,
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
	s.state = &dummyState{
		credentials: map[string]credential.ID{},
		modelCredentials: map[string]credential.ID{
			modelUUID.String(): {},
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

	svc := NewService(modelUUID, s.state)
	err = svc.SetCloudCredential(context.Background(), tag)
	c.Assert(err, jc.ErrorIsNil)

	foundCred, exists := s.state.modelCredentials[modelUUID.String()]
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

	svc := NewService("noexist", s.state)
	err = svc.SetCloudCredential(context.Background(), tag)
	c.Assert(err, jc.ErrorIs, modelerrors.NotFound)
}

func (s *serviceSuite) TestSetModelCredentialNotFound(c *C) {
	tag, err := names.ParseCloudCredentialTag("cloudcred-testcloud_wallyworld_ipv6world")
	c.Assert(err, jc.ErrorIsNil)

	svc := NewService(modelUUID, s.state)
	err = svc.SetCloudCredential(context.Background(), tag)
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

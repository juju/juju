// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	corecredential "github.com/juju/juju/core/credential"
	credentialtesting "github.com/juju/juju/core/credential/testing"
	usertesting "github.com/juju/juju/core/user/testing"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain/credential"
)

type providerServiceSuite struct {
	baseSuite
}

var _ = gc.Suite(&providerServiceSuite{})

func (s *providerServiceSuite) service() *WatchableProviderService {
	return NewWatchableProviderService(s.state, s.watcherFactory)
}

func (s *providerServiceSuite) TestCloudCredential(c *gc.C) {
	defer s.setupMocks(c).Finish()

	key := corecredential.Key{Cloud: "cirrus", Owner: usertesting.GenNewName(c, "fred"), Name: "foo"}
	cred := credential.CloudCredentialResult{
		CloudCredentialInfo: credential.CloudCredentialInfo{
			AuthType: string(cloud.UserPassAuthType),
			Attributes: map[string]string{
				"hello": "world",
			},
			Label: "foo",
		},
	}
	s.state.EXPECT().CloudCredential(gomock.Any(), key).Return(cred, nil)

	result, err := s.service().CloudCredential(context.Background(), key)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, cloud.NewNamedCredential("foo", cloud.UserPassAuthType, map[string]string{"hello": "world"}, false))
}

func (s *providerServiceSuite) TestCloudCredentialInvalidKey(c *gc.C) {
	defer s.setupMocks(c).Finish()

	key := corecredential.Key{Cloud: "cirrus", Owner: usertesting.GenNewName(c, "fred")}
	_, err := s.service().CloudCredential(context.Background(), key)
	c.Assert(err, gc.ErrorMatches, "invalid id getting cloud credential.*")
}

func (s *providerServiceSuite) TestWatchCredential(c *gc.C) {
	defer s.setupMocks(c).Finish()

	nw := watchertest.NewMockNotifyWatcher(nil)

	key := corecredential.Key{Cloud: "cirrus", Owner: usertesting.GenNewName(c, "fred"), Name: "foo"}
	s.state.EXPECT().WatchCredential(gomock.Any(), gomock.Any(), key).Return(nw, nil)

	w, err := s.service().WatchCredential(context.Background(), key)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(w, gc.NotNil)
}

func (s *providerServiceSuite) TestWatchCredentialInvalidKey(c *gc.C) {
	defer s.setupMocks(c).Finish()

	key := corecredential.Key{Cloud: "cirrus", Owner: usertesting.GenNewName(c, "fred")}
	_, err := s.service().WatchCredential(context.Background(), key)
	c.Assert(err, gc.ErrorMatches, "watching cloud credential with invalid key.*")
}

func (s *providerServiceSuite) TestInvalidateCredential(c *gc.C) {
	defer s.setupMocks(c).Finish()

	uuid := credentialtesting.GenCredentialUUID(c)
	key := corecredential.Key{Cloud: "cirrus", Owner: usertesting.GenNewName(c, "fred"), Name: "foo"}
	s.state.EXPECT().CredentialUUIDForKey(gomock.Any(), key).Return(uuid, nil)
	s.state.EXPECT().InvalidateCloudCredential(gomock.Any(), uuid, "bad")

	err := s.service().InvalidateCredential(context.Background(), key, "bad")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *providerServiceSuite) TestInvalidateCredentialInvalidKey(c *gc.C) {
	defer s.setupMocks(c).Finish()

	key := corecredential.Key{Cloud: "cirrus", Owner: usertesting.GenNewName(c, "fred")}
	err := s.service().InvalidateCredential(context.Background(), key, "bad")
	c.Assert(err, gc.ErrorMatches, "invalidating cloud credential with invalid key.*")
}

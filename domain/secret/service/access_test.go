// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	coresecrets "github.com/juju/juju/core/secrets"
	domainsecret "github.com/juju/juju/domain/secret"
)

func (s *serviceSuite) TestCanManageOwnerUnit(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	uri := coresecrets.NewURI()

	s.state = NewMockState(ctrl)
	s.state.EXPECT().GetSecretAccess(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mariadb/0",
	}).Return("manage", nil)

	token := NewMockToken(ctrl)

	err := s.service().canManage(context.Background(), uri, SecretAccessor{
		Kind: UnitAccessor,
		ID:   "mariadb/0",
	}, token)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestCanManageLeaderUnitAppSecret(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	uri := coresecrets.NewURI()

	s.state = NewMockState(ctrl)
	s.state.EXPECT().GetSecretAccess(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mariadb/0",
	}).Return("none", nil)
	s.state.EXPECT().GetSecretAccess(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectApplication,
		SubjectID:     "mariadb",
	}).Return("manage", nil)

	token := NewMockToken(ctrl)
	token.EXPECT().Check().Return(nil)

	err := s.service().canManage(context.Background(), uri, SecretAccessor{
		Kind: UnitAccessor,
		ID:   "mariadb/0",
	}, token)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestCanManageUserSecrets(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	uri := coresecrets.NewURI()

	s.state = NewMockState(ctrl)
	s.state.EXPECT().GetSecretAccess(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectModel,
		SubjectID:     "model-uuid",
	}).Return("manage", nil)

	token := NewMockToken(ctrl)

	err := s.service().canManage(context.Background(), uri, SecretAccessor{
		Kind: ModelAccessor,
		ID:   "model-uuid",
	}, token)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestCanReadAppSecret(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	uri := coresecrets.NewURI()

	s.state = NewMockState(ctrl)
	s.state.EXPECT().GetSecretAccess(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mariadb/0",
	}).Return("none", nil)
	s.state.EXPECT().GetSecretAccess(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectApplication,
		SubjectID:     "mariadb",
	}).Return("view", nil)

	err := s.service().canRead(context.Background(), uri, SecretAccessor{
		Kind: UnitAccessor,
		ID:   "mariadb/0",
	})
	c.Assert(err, jc.ErrorIsNil)
}

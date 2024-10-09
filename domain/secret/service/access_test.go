// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	coresecrets "github.com/juju/juju/core/secrets"
	domainsecret "github.com/juju/juju/domain/secret"
)

func (s *serviceSuite) TestCanManageOwnerUnit(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	uri := coresecrets.NewURI()

	s.state.EXPECT().GetSecretAccess(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mariadb/0",
	}).Return("manage", nil)

	err := s.service.canManage(NewMockAtomicContext(ctrl), false, uri, SecretAccessor{
		Kind: UnitAccessor,
		ID:   "mariadb/0",
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestCanManageLeaderUnitAppSecret(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	uri := coresecrets.NewURI()

	s.state.EXPECT().GetSecretAccess(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mariadb/0",
	}).Return("none", nil)
	s.state.EXPECT().GetSecretAccess(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectApplication,
		SubjectID:     "mariadb",
	}).Return("manage", nil)

	err := s.service.canManage(NewMockAtomicContext(ctrl), true, uri, SecretAccessor{
		Kind: UnitAccessor,
		ID:   "mariadb/0",
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestCanManageUserSecrets(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	uri := coresecrets.NewURI()

	s.state.EXPECT().GetSecretAccess(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectModel,
		SubjectID:     "model-uuid",
	}).Return("manage", nil)

	err := s.service.canManage(NewMockAtomicContext(ctrl), false, uri, SecretAccessor{
		Kind: ModelAccessor,
		ID:   "model-uuid",
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestCanReadAppSecret(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	uri := coresecrets.NewURI()

	s.state.EXPECT().GetSecretAccess(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mariadb/0",
	}).Return("none", nil)
	s.state.EXPECT().GetSecretAccess(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectApplication,
		SubjectID:     "mariadb",
	}).Return("view", nil)

	err := s.service.canRead(NewMockAtomicContext(ctrl), uri, SecretAccessor{
		Kind: UnitAccessor,
		ID:   "mariadb/0",
	})
	c.Assert(err, jc.ErrorIsNil)
}

// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsmanager_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coresecrets "github.com/juju/juju/core/secrets"
)

func (s *SecretsManagerSuite) TestCanManageOwnerUnit(c *gc.C) {
	s.authTag = names.NewUnitTag("mariadb/0")
	defer s.setup(c).Finish()

	uri := coresecrets.NewURI()
	gomock.InOrder(
		s.secretsConsumer.EXPECT().SecretAccess(uri, names.NewUnitTag("mariadb/0")).Return(coresecrets.RoleManage, nil),
	)

	token, err := s.facade.CanManage(uri)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(token.Check(), jc.ErrorIsNil)
}

func (s *SecretsManagerSuite) TestCanManageLeaderUnitAppSecret(c *gc.C) {
	s.authTag = names.NewUnitTag("mariadb/0")
	defer s.setup(c).Finish()

	uri := coresecrets.NewURI()
	gomock.InOrder(
		s.secretsConsumer.EXPECT().SecretAccess(uri, names.NewUnitTag("mariadb/0")).Return(coresecrets.RoleNone, errors.NotFoundf("")),
		s.secretsConsumer.EXPECT().SecretAccess(uri, names.NewApplicationTag("mariadb")).Return(coresecrets.RoleManage, nil),
		s.leadership.EXPECT().LeadershipCheck("mariadb", "mariadb/0").Return(s.token),
		s.token.EXPECT().Check().Return(nil),
	)

	_, err := s.facade.CanManage(uri)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *SecretsManagerSuite) TestCanManageAppTagLogin(c *gc.C) {
	s.authTag = names.NewApplicationTag("mariadb")
	defer s.setup(c).Finish()

	uri := coresecrets.NewURI()
	gomock.InOrder(
		s.secretsConsumer.EXPECT().SecretAccess(uri, names.NewApplicationTag("mariadb")).Return(coresecrets.RoleManage, nil),
	)

	token, err := s.facade.CanManage(uri)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(token.Check(), jc.ErrorIsNil)
}

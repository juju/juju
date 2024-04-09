// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets_test

import (
	"context"

	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common/secrets"
	"github.com/juju/juju/apiserver/common/secrets/mocks"
	coresecrets "github.com/juju/juju/core/secrets"
	secretservice "github.com/juju/juju/domain/secret/service"
	coretesting "github.com/juju/juju/testing"
)

func (s *secretsSuite) TestCanManageOwnerUnit(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	secretsConsumer := mocks.NewMockSecretConsumer(ctrl)
	leadershipChecker := mocks.NewMockChecker(ctrl)
	authTag := names.NewUnitTag("mariadb/0")

	uri := coresecrets.NewURI()
	secretsConsumer.EXPECT().GetSecretAccess(gomock.Any(), uri, secretservice.SecretAccessor{
		Kind: secretservice.UnitAccessor, ID: "mariadb/0",
	}).Return(coresecrets.RoleManage, nil)

	t, err := secrets.CanManage(context.Background(), secretsConsumer, leadershipChecker, authTag, uri)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(t.Check(), jc.ErrorIsNil)
}

func (s *secretsSuite) TestCanManageLeaderUnitAppSecret(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	secretsConsumer := mocks.NewMockSecretConsumer(ctrl)
	leadershipChecker := mocks.NewMockChecker(ctrl)
	token := mocks.NewMockToken(ctrl)
	authTag := names.NewUnitTag("mariadb/0")

	uri := coresecrets.NewURI()
	secretsConsumer.EXPECT().GetSecretAccess(gomock.Any(), uri, secretservice.SecretAccessor{
		Kind: secretservice.UnitAccessor, ID: "mariadb/0",
	}).Return(coresecrets.RoleNone, nil)
	secretsConsumer.EXPECT().GetSecretAccess(gomock.Any(), uri, secretservice.SecretAccessor{
		Kind: secretservice.ApplicationAccessor, ID: "mariadb",
	}).Return(coresecrets.RoleManage, nil)
	leadershipChecker.EXPECT().LeadershipCheck("mariadb", "mariadb/0").Return(token)
	token.EXPECT().Check().Return(nil)

	_, err := secrets.CanManage(context.Background(), secretsConsumer, leadershipChecker, authTag, uri)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *secretsSuite) TestCanManageUserSecrets(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	secretsConsumer := mocks.NewMockSecretConsumer(ctrl)
	leadershipChecker := mocks.NewMockChecker(ctrl)
	authTag := coretesting.ModelTag

	uri := coresecrets.NewURI()
	secretsConsumer.EXPECT().GetSecretAccess(gomock.Any(), uri, secretservice.SecretAccessor{
		Kind: secretservice.ModelAccessor, ID: coretesting.ModelTag.Id(),
	}).Return(coresecrets.RoleManage, nil)

	t, err := secrets.CanManage(context.Background(), secretsConsumer, leadershipChecker, authTag, uri)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(t.Check(), jc.ErrorIsNil)
}

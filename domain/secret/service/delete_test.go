// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coresecrets "github.com/juju/juju/core/secrets"
	domainsecret "github.com/juju/juju/domain/secret"
)

func (s *serviceSuite) TestDeleteObsoleteUserSecretRevisions(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.state.EXPECT().ScheduleObsoleteUserSecretRevisionsPruning(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

	err := s.service.DeleteObsoleteUserSecretRevisions(c.Context())
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestDeleteSecret(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	uri := coresecrets.NewURI()

	s.state.EXPECT().GetSecretAccess(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mariadb/0",
	}).Return("manage", nil)
	s.state.EXPECT().GetSecretRevisionID(gomock.Any(), uri, 1).Return("", nil)
	s.state.EXPECT().GetSecretRevisionID(gomock.Any(), uri, 2).Return("", nil)
	s.state.EXPECT().ScheduleUserSecretRemoval(gomock.Any(), gomock.Any(), uri, []int{1, 2}, gomock.Any()).Return(nil)

	err := s.service.DeleteSecret(c.Context(), uri, domainsecret.DeleteSecretParams{
		Accessor: domainsecret.SecretAccessor{
			Kind: domainsecret.UnitAccessor,
			ID:   "mariadb/0",
		},
		Revisions: []int{1, 2},
	})
	c.Assert(err, tc.ErrorIsNil)
}

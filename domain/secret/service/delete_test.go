// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	coresecrets "github.com/juju/juju/core/secrets"
	domainsecret "github.com/juju/juju/domain/secret"
	"github.com/juju/juju/internal/secrets/provider"
)

func (s *serviceSuite) TestDeleteSecretInternal(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.backendConfigGetter = func(context.Context) (*provider.ModelBackendConfigInfo, error) {
		return &provider.ModelBackendConfigInfo{}, nil
	}

	uri := coresecrets.NewURI()

	s.state = NewMockState(ctrl)
	s.state.EXPECT().GetSecretAccess(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mariadb/0",
	}).Return("manage", nil)
	s.state.EXPECT().ListExternalSecretRevisions(gomock.Any(), uri, 666).Return([]coresecrets.ValueRef{}, nil)
	s.state.EXPECT().DeleteSecret(gomock.Any(), uri, []int{666}).Return(nil)

	err := s.service().DeleteSecret(context.Background(), uri, DeleteSecretParams{
		LeaderToken: successfulToken{},
		Accessor: SecretAccessor{
			Kind: UnitAccessor,
			ID:   "mariadb/0",
		},
		Revisions: []int{666},
	})
	c.Assert(err, jc.ErrorIsNil)
}

// TODO(secrets) - add tests for backend when properly implemented

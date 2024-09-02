// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/uuid"
)

func (s *serviceSuite) TestDeleteObsoleteUserSecretRevisions(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	revisionID1, err := uuid.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	revisionID2, err := uuid.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	s.state.EXPECT().DeleteObsoleteUserSecretRevisions(gomock.Any()).Return([]string{revisionID1.String(), revisionID2.String()}, nil)
	s.secretBackendReferenceMutator.EXPECT().RemoveSecretBackendReference(gomock.Any(), revisionID1.String(), revisionID2.String()).Return(nil)

	err = s.service.DeleteObsoleteUserSecretRevisions(context.Background())
	c.Assert(err, jc.ErrorIsNil)
}

// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/tc"

	coremodel "github.com/juju/juju/core/model"
	coremodelmigration "github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/domain/secretbackend"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/uuid"
)

type importSuite struct {
	testhelpers.IsolationSuite

	state *MockState
}

func TestImportSuite(t *testing.T) {
	tc.Run(t, &importSuite{})
}

func (s *importSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.state = NewMockState(ctrl)
	return ctrl
}

func (s *importSuite) TestImportSecretBackendReferences(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := tc.Must0(c, coremodel.NewUUID)
	backend := &secretbackend.SecretBackend{ID: uuid.MustNewUUID().String(), Name: "vault"}

	s.state.EXPECT().GetSecretBackend(gomock.Any(), secretbackend.BackendIdentifier{Name: "vault"}).
		Return(backend, nil)
	s.state.EXPECT().AddSecretBackendReference(
		gomock.Any(), &secrets.ValueRef{BackendID: backend.ID}, modelUUID, "rev-1", "secret:1",
	).Return(func() error { return nil }, nil)
	s.state.EXPECT().AddSecretBackendReference(
		gomock.Any(), &secrets.ValueRef{BackendID: backend.ID}, modelUUID, "rev-2", "secret:2",
	).Return(func() error { return nil }, nil)

	err := NewService(s.state, loggertesting.WrapCheckLog(c)).ImportSecretBackendReferences(
		c.Context(), modelUUID,
		[]coremodelmigration.SecretBackendReference{{
			BackendName:        "vault",
			SecretRevisionUUID: "rev-1",
			SecretID:           "secret:1",
		}, {
			BackendName:        "vault",
			SecretRevisionUUID: "rev-2",
			SecretID:           "secret:2",
		}},
	)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestImportSecretBackendReferencesEmpty(c *tc.C) {
	defer s.setupMocks(c).Finish()

	err := NewService(s.state, loggertesting.WrapCheckLog(c)).ImportSecretBackendReferences(
		c.Context(), tc.Must0(c, coremodel.NewUUID), nil,
	)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestImportSecretBackendReferencesLookupError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	expected := errors.New("boom")
	s.state.EXPECT().GetSecretBackend(gomock.Any(), secretbackend.BackendIdentifier{Name: "vault"}).
		Return(nil, expected)

	err := NewService(s.state, loggertesting.WrapCheckLog(c)).ImportSecretBackendReferences(
		c.Context(), tc.Must0(c, coremodel.NewUUID),
		[]coremodelmigration.SecretBackendReference{{BackendName: "vault"}},
	)
	c.Assert(err, tc.ErrorIs, expected)
}

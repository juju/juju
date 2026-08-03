// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"testing"

	gomock "github.com/canonical/gomock/gomock"
	"github.com/juju/tc"

	"github.com/juju/juju/cloud"
	coreerrors "github.com/juju/juju/core/errors"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/domain/model"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
)

type migrationServiceSuite struct {
	testhelpers.IsolationSuite

	state *MockState
}

func TestMigrationServiceSuite(t *testing.T) {
	tc.Run(t, &migrationServiceSuite{})
}

func (s *migrationServiceSuite) newService(c *tc.C) *MigrationService {
	return NewMigrationService(s.state, loggertesting.WrapCheckLog(c))
}

func (s *migrationServiceSuite) TestImportModelLegacyIAAS(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uuid := tc.Must(c, coremodel.NewUUID)

	sExp := s.state.EXPECT()
	sExp.CloudType(gomock.Any(), "aws").Return("aws", nil)
	sExp.ImportModel(gomock.Any(), uuid, coremodel.IAAS, gomock.Any()).Return(nil)

	svc := s.newService(c)

	err := svc.ImportModelLegacy(c.Context(), model.ModelImportArgs{
		UUID: uuid,
		GlobalModelCreationArgs: model.GlobalModelCreationArgs{
			Name:      "foo",
			Cloud:     "aws",
			Qualifier: coremodel.Qualifier("jim"),
		},
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *migrationServiceSuite) TestImportModelLegacyCAAS(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uuid := tc.Must(c, coremodel.NewUUID)

	sExp := s.state.EXPECT()
	sExp.CloudType(gomock.Any(), "k8s").Return(cloud.CloudTypeKubernetes, nil)
	sExp.ImportModel(gomock.Any(), uuid, coremodel.CAAS, gomock.Any()).Return(nil)

	svc := s.newService(c)

	err := svc.ImportModelLegacy(c.Context(), model.ModelImportArgs{
		UUID: uuid,
		GlobalModelCreationArgs: model.GlobalModelCreationArgs{
			Name:      "foo",
			Cloud:     "k8s",
			Qualifier: coremodel.Qualifier("jim"),
		},
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *migrationServiceSuite) TestImportModel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uuid := tc.Must(c, coremodel.NewUUID)

	sExp := s.state.EXPECT()
	sExp.CloudType(gomock.Any(), "aws").Return("aws", nil)
	// ImportModel must bootstrap via the claim-free Create path, never via
	// ImportModelLegacy -- the v8 import claim is owned by the modelmigration
	// domain and must not be duplicated here.
	sExp.Create(gomock.Any(), uuid, coremodel.IAAS, gomock.Any()).Return(nil)

	svc := s.newService(c)

	err := svc.ImportModel(c.Context(), model.ModelImportArgs{
		UUID: uuid,
		GlobalModelCreationArgs: model.GlobalModelCreationArgs{
			Name:      "foo",
			Cloud:     "aws",
			Qualifier: coremodel.Qualifier("jim"),
		},
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *migrationServiceSuite) TestImportModelCAAS(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uuid := tc.Must(c, coremodel.NewUUID)

	sExp := s.state.EXPECT()
	sExp.CloudType(gomock.Any(), "k8s").Return(cloud.CloudTypeKubernetes, nil)
	sExp.Create(gomock.Any(), uuid, coremodel.CAAS, gomock.Any()).Return(nil)

	svc := s.newService(c)

	err := svc.ImportModel(c.Context(), model.ModelImportArgs{
		UUID: uuid,
		GlobalModelCreationArgs: model.GlobalModelCreationArgs{
			Name:      "foo",
			Cloud:     "k8s",
			Qualifier: coremodel.Qualifier("jim"),
		},
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *migrationServiceSuite) TestImportModelValidationFails(c *tc.C) {
	defer s.setupMocks(c).Finish()

	svc := s.newService(c)

	err := svc.ImportModel(c.Context(), model.ModelImportArgs{
		UUID: "not valid",
		GlobalModelCreationArgs: model.GlobalModelCreationArgs{
			Cloud:     "aws",
			Name:      "foo",
			Qualifier: coremodel.Qualifier("jim"),
		},
	})
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *migrationServiceSuite) TestImportModelLegacyActivate(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uuid := tc.Must(c, coremodel.NewUUID)

	sExp := s.state.EXPECT()
	sExp.CloudType(gomock.Any(), gomock.Any()).Return("aws", nil)
	sExp.ImportModel(gomock.Any(), uuid, gomock.Any(), gomock.Any()).Return(nil)
	sExp.Activate(gomock.Any(), uuid).Return(nil)

	svc := s.newService(c)

	err := svc.ImportModelLegacy(c.Context(), model.ModelImportArgs{
		UUID: uuid,
		GlobalModelCreationArgs: model.GlobalModelCreationArgs{
			Name:      "foo",
			Cloud:     "aws",
			Qualifier: coremodel.Qualifier("jim"),
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	err = svc.ActivateModel(c.Context(), uuid)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *migrationServiceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)

	c.Cleanup(func() {
		s.state = nil
	})

	return ctrl
}

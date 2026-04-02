// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"testing"

	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	"github.com/juju/juju/cloud"
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

func (s *migrationServiceSuite) TestImportModelIAAS(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uuid := tc.Must(c, coremodel.NewUUID)

	sExp := s.state.EXPECT()
	sExp.CloudType(gomock.Any(), "aws").Return("aws", nil)
	sExp.ImportModel(gomock.Any(), uuid, coremodel.IAAS, gomock.Any()).Return(nil)

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
	sExp.ImportModel(gomock.Any(), uuid, coremodel.CAAS, gomock.Any()).Return(nil)

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

func (s *migrationServiceSuite) TestImportModelActivate(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uuid := tc.Must(c, coremodel.NewUUID)

	sExp := s.state.EXPECT()
	sExp.CloudType(gomock.Any(), gomock.Any()).Return("aws", nil)
	sExp.ImportModel(gomock.Any(), uuid, gomock.Any(), gomock.Any()).Return(nil)
	sExp.Activate(gomock.Any(), uuid).Return(nil)

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

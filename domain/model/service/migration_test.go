// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/credential"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/user"
	usertesting "github.com/juju/juju/core/user/testing"
	"github.com/juju/juju/domain/model"
	modelerrors "github.com/juju/juju/domain/model/errors"
	changestreamtesting "github.com/juju/juju/internal/changestream/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	jujusecrets "github.com/juju/juju/internal/secrets/provider/juju"
	kubernetessecrets "github.com/juju/juju/internal/secrets/provider/kubernetes"
	"github.com/juju/juju/internal/testhelpers"
)

const deletedID = "7a53b695-4cb6-4c1d-86da-c2dca65ed957"

type migrationServiceSuite struct {
	testhelpers.IsolationSuite

	userUUID user.UUID
	state    *dummyState
	deleter  *dummyDeleter

	changestreamtesting.ControllerSuite
}

func TestMigrationServiceSuite(t *testing.T) {
	tc.Run(t, &migrationServiceSuite{})
}

func (s *migrationServiceSuite) SetUpTest(c *tc.C) {
	var err error
	s.userUUID = usertesting.GenUserUUID(c)
	c.Assert(err, tc.ErrorIsNil)
	s.state = &dummyState{
		clouds:             map[string]dummyStateCloud{},
		models:             map[coremodel.UUID]coremodel.Model{},
		nonActivatedModels: map[coremodel.UUID]coremodel.Model{},
		users: map[user.UUID]user.Name{
			s.userUUID: user.AdminUserName,
		},
		secretBackends: []string{
			jujusecrets.BackendName,
			kubernetessecrets.BackendName,
		},
	}
	s.deleter = &dummyDeleter{
		deleted: map[string]struct{}{},
	}
}

func (s *migrationServiceSuite) newService(c *tc.C) *MigrationService {
	return NewMigrationService(s.state, s.deleter, loggertesting.WrapCheckLog(c))
}

// TestImportModel is asserting the happy path for importing a model.
func (s *migrationServiceSuite) TestImportModel(c *tc.C) {
	cred := credential.Key{
		Cloud: "aws",
		Name:  "foobar",
		Owner: usertesting.GenNewName(c, "owner"),
	}
	s.state.clouds["aws"] = dummyStateCloud{
		Credentials: map[string]credential.Key{
			cred.String(): cred,
		},
		Regions: []string{"myregion"},
	}

	modelID := modeltesting.GenModelUUID(c)

	svc := s.newService(c)
	activator, err := svc.ImportModel(c.Context(), model.ModelImportArgs{
		GlobalModelCreationArgs: model.GlobalModelCreationArgs{
			Cloud:       "aws",
			CloudRegion: "myregion",
			Credential:  cred,
			AdminUsers:  []user.UUID{s.userUUID},
			Name:        "my-awesome-model",
			Qualifier:   "prod",
		},
		UUID: modelID,
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(activator(c.Context()), tc.ErrorIsNil)

	_, exists := s.state.models[modelID]
	c.Assert(exists, tc.IsTrue)
}

func (s *migrationServiceSuite) TestDeleteModel(c *tc.C) {
	cred := credential.Key{
		Cloud: "aws",
		Name:  "foobar",
		Owner: usertesting.GenNewName(c, "owner"),
	}
	s.state.clouds["aws"] = dummyStateCloud{
		Credentials: map[string]credential.Key{
			cred.String(): cred,
		},
		Regions: []string{"myregion"},
	}

	id := modeltesting.GenModelUUID(c)

	svc := s.newService(c)
	activator, err := svc.ImportModel(c.Context(), model.ModelImportArgs{
		GlobalModelCreationArgs: model.GlobalModelCreationArgs{
			Cloud:       "aws",
			CloudRegion: "myregion",
			Credential:  cred,
			AdminUsers:  []user.UUID{s.userUUID},
			Name:        "my-awesome-model",
			Qualifier:   "prod",
		},
		UUID: id,
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(activator(c.Context()), tc.ErrorIsNil)

	_, exists := s.state.models[id]
	c.Assert(exists, tc.IsTrue)

	err = svc.DeleteModel(c.Context(), id, model.WithDeleteDB())
	c.Assert(err, tc.ErrorIsNil)
	_, exists = s.state.models[id]
	c.Assert(exists, tc.IsFalse)

	_, exists = s.deleter.deleted[id.String()]
	c.Assert(exists, tc.IsTrue)
}

func (s *migrationServiceSuite) TestDeleteModelNotFound(c *tc.C) {
	svc := s.newService(c)

	err := svc.DeleteModel(c.Context(), deletedID, model.WithDeleteDB())
	c.Assert(err, tc.ErrorIs, modelerrors.NotFound)
}

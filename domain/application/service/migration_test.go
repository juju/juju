// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"testing"

	"github.com/juju/collections/set"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coreapplication "github.com/juju/juju/core/application"
	applicationtesting "github.com/juju/juju/core/application/testing"
	corecharm "github.com/juju/juju/core/charm"
	charmtesting "github.com/juju/juju/core/charm/testing"
	"github.com/juju/juju/core/config"
	"github.com/juju/juju/core/constraints"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/unit"
	unittesting "github.com/juju/juju/core/unit/testing"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/architecture"
	domaincharm "github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	domainconstraints "github.com/juju/juju/domain/constraints"
	"github.com/juju/juju/domain/deployment"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type migrationServiceSuite struct {
	baseSuite

	service *MigrationService
}

func TestMigrationServiceSuite(t *testing.T) {
	tc.Run(t, &migrationServiceSuite{})
}

func (s *migrationServiceSuite) TestGetCharmIDWithoutRevision(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service.GetCharmID(c.Context(), domaincharm.GetCharmArgs{
		Name:   "foo",
		Source: domaincharm.CharmHubSource,
	})
	c.Assert(err, tc.ErrorIs, applicationerrors.CharmNotFound)
}

func (s *migrationServiceSuite) TestGetCharmIDWithoutSource(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service.GetCharmID(c.Context(), domaincharm.GetCharmArgs{
		Name:     "foo",
		Revision: ptr(42),
	})
	c.Assert(err, tc.ErrorIs, applicationerrors.CharmSourceNotValid)
}

func (s *migrationServiceSuite) TestGetCharmIDInvalidName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service.GetCharmID(c.Context(), domaincharm.GetCharmArgs{
		Name: "Foo",
	})
	c.Assert(err, tc.ErrorIs, applicationerrors.CharmNameNotValid)
}

func (s *migrationServiceSuite) TestGetCharmIDInvalidSource(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service.GetCharmID(c.Context(), domaincharm.GetCharmArgs{
		Name:     "foo",
		Revision: ptr(42),
		Source:   "wrong-source",
	})
	c.Assert(err, tc.ErrorIs, applicationerrors.CharmSourceNotValid)
}

func (s *migrationServiceSuite) TestGetCharmID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	rev := 42

	s.state.EXPECT().GetCharmID(gomock.Any(), "foo", rev, domaincharm.LocalSource).Return(id, nil)

	result, err := s.service.GetCharmID(c.Context(), domaincharm.GetCharmArgs{
		Name:     "foo",
		Revision: &rev,
		Source:   domaincharm.LocalSource,
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.Equals, id)
}

func (s *migrationServiceSuite) TestGetCharm(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Conversion of the metadata tests is done in the types package.

	id := charmtesting.GenCharmID(c)

	s.state.EXPECT().GetCharmIDByApplicationName(gomock.Any(), "foo").Return(id, nil)
	s.state.EXPECT().GetCharm(gomock.Any(), id).Return(domaincharm.Charm{
		Metadata: domaincharm.Metadata{
			Name: "foo",

			// RunAs becomes mandatory when being persisted. Empty string is not
			// allowed.
			RunAs: "default",
		},
		Source:    domaincharm.LocalSource,
		Revision:  42,
		Available: true,
	}, nil, nil)

	metadata, locator, err := s.service.GetCharmByApplicationName(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(metadata.Meta(), tc.DeepEquals, &charm.Meta{
		Name: "foo",

		// Notice that the RunAs field becomes empty string when being returned.
	})
	c.Check(locator, tc.Equals, domaincharm.CharmLocator{
		Source:   domaincharm.LocalSource,
		Revision: 42,
	})
}

func (s *migrationServiceSuite) TestGetCharmInvalidMetadata(c *tc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	s.state.EXPECT().GetCharmIDByApplicationName(gomock.Any(), "foo").Return(id, nil)
	s.state.EXPECT().GetCharm(gomock.Any(), id).Return(domaincharm.Charm{
		Metadata: domaincharm.Metadata{
			Name:  "foo",
			RunAs: "blah",
		},
		Source:    domaincharm.LocalSource,
		Revision:  42,
		Available: true,
	}, nil, nil)

	_, _, err := s.service.GetCharmByApplicationName(c.Context(), "foo")
	c.Assert(err, tc.ErrorMatches, `.*decode charm user.*`)
}

func (s *migrationServiceSuite) TestGetCharmInvalidManifest(c *tc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	s.state.EXPECT().GetCharmIDByApplicationName(gomock.Any(), "foo").Return(id, nil)
	s.state.EXPECT().GetCharm(gomock.Any(), id).Return(domaincharm.Charm{
		Metadata: domaincharm.Metadata{
			Name:  "foo",
			RunAs: "default",
		},
		Manifest: domaincharm.Manifest{
			Bases: []domaincharm.Base{
				{
					Name: "foo",
				},
			},
		},
		Source:    domaincharm.LocalSource,
		Revision:  42,
		Available: true,
	}, nil, nil)

	_, _, err := s.service.GetCharmByApplicationName(c.Context(), "foo")
	c.Assert(err, tc.ErrorMatches, `.*decode bases: decode base.*`)
}

func (s *migrationServiceSuite) TestGetCharmInvalidActions(c *tc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	s.state.EXPECT().GetCharmIDByApplicationName(gomock.Any(), "foo").Return(id, nil)
	s.state.EXPECT().GetCharm(gomock.Any(), id).Return(domaincharm.Charm{
		Metadata: domaincharm.Metadata{
			Name:  "foo",
			RunAs: "default",
		},
		Actions: domaincharm.Actions{
			Actions: map[string]domaincharm.Action{
				"foo": {
					Params: []byte("!!!"),
				},
			},
		},
		Source:    domaincharm.LocalSource,
		Revision:  42,
		Available: true,
	}, nil, nil)

	_, _, err := s.service.GetCharmByApplicationName(c.Context(), "foo")
	c.Assert(err, tc.ErrorMatches, `.*decode action params: unmarshal.*`)
}

func (s *migrationServiceSuite) TestGetCharmInvalidConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	s.state.EXPECT().GetCharmIDByApplicationName(gomock.Any(), "foo").Return(id, nil)
	s.state.EXPECT().GetCharm(gomock.Any(), id).Return(domaincharm.Charm{
		Metadata: domaincharm.Metadata{
			Name:  "foo",
			RunAs: "default",
		},
		Config: domaincharm.Config{
			Options: map[string]domaincharm.Option{
				"foo": {
					Type: "foo",
				},
			},
		},
		Source:    domaincharm.LocalSource,
		Revision:  42,
		Available: true,
	}, nil, nil)

	_, _, err := s.service.GetCharmByApplicationName(c.Context(), "foo")
	c.Assert(err, tc.ErrorMatches, `.*decode config.*`)
}

func (s *migrationServiceSuite) TestGetCharmInvalidLXDProfile(c *tc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	s.state.EXPECT().GetCharmIDByApplicationName(gomock.Any(), "foo").Return(id, nil)
	s.state.EXPECT().GetCharm(gomock.Any(), id).Return(domaincharm.Charm{
		Metadata: domaincharm.Metadata{
			Name:  "foo",
			RunAs: "default",
		},
		LXDProfile: []byte("!!!"),
		Source:     domaincharm.LocalSource,
		Revision:   42,
		Available:  true,
	}, nil, nil)

	_, _, err := s.service.GetCharmByApplicationName(c.Context(), "foo")
	c.Assert(err, tc.ErrorMatches, `.*unmarshal lxd profile.*`)
}

func (s *migrationServiceSuite) TestGetCharmCharmNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	s.state.EXPECT().GetCharmIDByApplicationName(gomock.Any(), "foo").Return(id, nil)
	s.state.EXPECT().GetCharm(gomock.Any(), id).Return(domaincharm.Charm{}, nil, applicationerrors.CharmNotFound)

	_, _, err := s.service.GetCharmByApplicationName(c.Context(), "foo")
	c.Assert(err, tc.ErrorIs, applicationerrors.CharmNotFound)
}

func (s *migrationServiceSuite) TestGetCharmInvalidUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, _, err := s.service.GetCharmByApplicationName(c.Context(), "")
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNameNotValid)
}

func (s *migrationServiceSuite) TestGetApplicationCharmOrigin(c *tc.C) {
	defer s.setupMocks(c).Finish()

	id := applicationtesting.GenApplicationUUID(c)

	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return(id, nil)
	s.state.EXPECT().GetApplicationCharmOrigin(gomock.Any(), id).Return(application.CharmOrigin{
		Name:   "foo",
		Source: domaincharm.CharmHubSource,
	}, nil)

	origin, err := s.service.GetApplicationCharmOrigin(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(origin, tc.DeepEquals, application.CharmOrigin{
		Name:   "foo",
		Source: domaincharm.CharmHubSource,
	})
}

func (s *migrationServiceSuite) TestGetApplicationCharmOriginGetApplicationError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	id := applicationtesting.GenApplicationUUID(c)

	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return(id, errors.Errorf("boom"))

	_, err := s.service.GetApplicationCharmOrigin(c.Context(), "foo")
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *migrationServiceSuite) TestGetApplicationCharmOriginInvalidID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service.GetApplicationCharmOrigin(c.Context(), "!!!!!!!!!!!")
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNameNotValid)
}

func (s *migrationServiceSuite) TestGetApplicationConfigAndSettings(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)

	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return(appUUID, nil)
	s.state.EXPECT().GetApplicationConfigAndSettings(gomock.Any(), appUUID).Return(map[string]application.ApplicationConfig{
		"foo": {
			Type:  domaincharm.OptionString,
			Value: "bar",
		},
	}, application.ApplicationSettings{
		Trust: true,
	}, nil)

	results, settings, err := s.service.GetApplicationConfigAndSettings(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(results, tc.DeepEquals, config.ConfigAttributes{
		"foo": "bar",
	})
	c.Check(settings, tc.DeepEquals, application.ApplicationSettings{
		Trust: true,
	})
}

func (s *migrationServiceSuite) TestGetApplicationConfigWithNameError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)

	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return(appUUID, errors.Errorf("boom"))

	_, _, err := s.service.GetApplicationConfigAndSettings(c.Context(), "foo")
	c.Assert(err, tc.ErrorMatches, "boom")

}

func (s *migrationServiceSuite) TestGetApplicationConfigWithConfigError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)

	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return(appUUID, nil)
	s.state.EXPECT().GetApplicationConfigAndSettings(gomock.Any(), appUUID).
		Return(map[string]application.ApplicationConfig{}, application.ApplicationSettings{}, errors.Errorf("boom"))

	_, _, err := s.service.GetApplicationConfigAndSettings(c.Context(), "foo")
	c.Assert(err, tc.ErrorMatches, "boom")

}

func (s *migrationServiceSuite) TestGetApplicationConfigNoConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)

	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return(appUUID, nil)
	s.state.EXPECT().GetApplicationConfigAndSettings(gomock.Any(), appUUID).
		Return(map[string]application.ApplicationConfig{}, application.ApplicationSettings{}, nil)

	results, settings, err := s.service.GetApplicationConfigAndSettings(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(results, tc.DeepEquals, config.ConfigAttributes{})
	c.Check(settings, tc.DeepEquals, application.ApplicationSettings{})
}

func (s *migrationServiceSuite) TestGetApplicationConfigNoConfigWithTrust(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)

	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return(appUUID, nil)
	s.state.EXPECT().GetApplicationConfigAndSettings(gomock.Any(), appUUID).
		Return(map[string]application.ApplicationConfig{}, application.ApplicationSettings{
			Trust: true,
		}, nil)

	results, settings, err := s.service.GetApplicationConfigAndSettings(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(results, tc.DeepEquals, config.ConfigAttributes{})
	c.Check(settings, tc.DeepEquals, application.ApplicationSettings{
		Trust: true,
	})
}

func (s *migrationServiceSuite) TestGetApplicationConfigInvalidApplicationName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, _, err := s.service.GetApplicationConfigAndSettings(c.Context(), "!!!")
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNameNotValid)
}

func (s *migrationServiceSuite) TestImportIAASApplication(c *tc.C) {
	s.assertImportApplication(c, coremodel.IAAS)
}

func (s *migrationServiceSuite) TestImportCAASApplication(c *tc.C) {
	s.assertImportApplication(c, coremodel.CAAS)
}

func (s *migrationServiceSuite) assertImportApplication(c *tc.C, modelType coremodel.ModelType) {
	defer s.setupMocks(c).Finish()

	id := applicationtesting.GenApplicationUUID(c)
	charmUUID := charmtesting.GenCharmID(c)

	ch := domaincharm.Charm{
		Metadata: domaincharm.Metadata{
			Name:  "ubuntu",
			RunAs: "default",
		},
		Manifest: s.minimalManifest(),
		Config: domaincharm.Config{
			Options: map[string]domaincharm.Option{
				"foo": {
					Type:    domaincharm.OptionString,
					Default: "baz",
				},
			},
		},
		ReferenceName: "ubuntu",
		Source:        domaincharm.CharmHubSource,
		Revision:      42,
		Architecture:  architecture.ARM64,
	}
	platform := deployment.Platform{
		Channel:      "24.04",
		OSType:       deployment.Ubuntu,
		Architecture: architecture.ARM64,
	}

	var receivedUnitArgs []application.ImportUnitArg
	if modelType == coremodel.IAAS {
		s.state.EXPECT().InsertMigratingIAASUnits(gomock.Any(), id, gomock.Any()).DoAndReturn(func(_ context.Context, _ coreapplication.ID, args ...application.ImportUnitArg) error {
			receivedUnitArgs = args
			return nil
		})
	} else {
		s.state.EXPECT().SetDesiredApplicationScale(gomock.Any(), id, 1).Return(nil)
		s.state.EXPECT().SetApplicationScalingState(gomock.Any(), "ubuntu", 42, true).Return(nil)
		s.state.EXPECT().InsertMigratingCAASUnits(gomock.Any(), id, gomock.Any()).DoAndReturn(func(_ context.Context, _ coreapplication.ID, args ...application.ImportUnitArg) error {
			receivedUnitArgs = args
			return nil
		})
	}
	s.charm.EXPECT().Actions().Return(&charm.Actions{})
	s.charm.EXPECT().Config().Return(&charm.Config{
		Options: map[string]charm.Option{
			"foo": {
				Type:    "string",
				Default: "baz",
			},
		},
	})
	s.charm.EXPECT().Meta().Return(&charm.Meta{
		Name: "ubuntu",
	}).MinTimes(1)
	s.charm.EXPECT().Manifest().Return(&charm.Manifest{
		Bases: []charm.Base{
			{
				Name: "ubuntu",
				Channel: charm.Channel{
					Risk: charm.Stable,
				},
				Architectures: []string{"amd64"},
			},
		},
	}).MinTimes(1)

	args := application.InsertApplicationArgs{
		Charm:    ch,
		Platform: platform,
		Scale:    1,
		Config: map[string]application.ApplicationConfig{
			"foo": {
				Type:  domaincharm.OptionString,
				Value: "bar",
			},
		},
		Settings: application.ApplicationSettings{
			Trust: true,
		},
	}
	s.state.EXPECT().InsertMigratingApplication(gomock.Any(), "ubuntu", args).Return(id, nil)

	unitArg := ImportUnitArg{
		UnitName:       "ubuntu/666",
		PasswordHash:   ptr("passwordhash"),
		CloudContainer: nil,
		Principal:      "principal/0",
	}

	cons := constraints.Value{
		Mem:      ptr(uint64(1024)),
		CpuPower: ptr(uint64(1000)),
		CpuCores: ptr(uint64(2)),
		Arch:     ptr("arm64"),
		Tags:     ptr([]string{"foo", "bar"}),
	}

	s.state.EXPECT().SetApplicationConstraints(gomock.Any(), id, domainconstraints.DecodeConstraints(cons)).Return(nil)

	s.state.EXPECT().GetCharmIDByApplicationName(gomock.Any(), "ubuntu").Return(charmUUID, nil)
	s.state.EXPECT().MergeExposeSettings(gomock.Any(), id, map[string]application.ExposedEndpoint{
		"": {
			ExposeToSpaceIDs: set.NewStrings("alpha"),
		},
		"endpoint0": {
			ExposeToCIDRs:    set.NewStrings("10.0.0.0/24", "10.0.1.0/24"),
			ExposeToSpaceIDs: set.NewStrings("space0", "space1"),
		},
	}).Return(nil)

	var importFunc func(ctx context.Context, name string, args ImportApplicationArgs) error
	if modelType == coremodel.IAAS {
		importFunc = s.service.ImportIAASApplication
	} else {
		importFunc = s.service.ImportCAASApplication
	}

	err := importFunc(c.Context(), "ubuntu", ImportApplicationArgs{
		Charm: s.charm,
		CharmOrigin: corecharm.Origin{
			Source:   corecharm.CharmHub,
			Platform: corecharm.MustParsePlatform("arm64/ubuntu/24.04"),
			Revision: ptr(42),
		},
		ApplicationConstraints: cons,
		ReferenceName:          "ubuntu",
		ApplicationConfig: map[string]any{
			"foo": "bar",
		},
		ApplicationSettings: application.ApplicationSettings{
			Trust: true,
		},
		Units: []ImportUnitArg{
			unitArg,
		},
		ScaleState: application.ScaleState{
			Scale:       1,
			Scaling:     true,
			ScaleTarget: 42,
		},
		ExposedEndpoints: map[string]application.ExposedEndpoint{
			"": {
				ExposeToSpaceIDs: set.NewStrings("alpha"),
			},
			"endpoint0": {
				ExposeToCIDRs:    set.NewStrings("10.0.0.0/24", "10.0.1.0/24"),
				ExposeToSpaceIDs: set.NewStrings("space0", "space1"),
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	expectedUnitArgs := []application.ImportUnitArg{{
		UnitName:       "ubuntu/666",
		CloudContainer: nil,
		Password: ptr(application.PasswordInfo{
			PasswordHash:  "passwordhash",
			HashAlgorithm: 0,
		}),
		Principal: "principal/0",
	}}
	c.Check(receivedUnitArgs, tc.DeepEquals, expectedUnitArgs)
}

func (s *migrationServiceSuite) TestRemoveImportedApplication(c *tc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service.RemoveImportedApplication(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *migrationServiceSuite) TestGetUnitUUIDByName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uuid := unittesting.GenUnitUUID(c)

	s.state.EXPECT().GetUnitUUIDByName(gomock.Any(), unit.Name("foo/0")).Return(uuid, nil)

	got, err := s.service.GetUnitUUIDByName(c.Context(), unit.Name("foo/0"))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got, tc.Equals, uuid)
}

func (s *migrationServiceSuite) TestGetUnitUUIDByNameInvalidName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service.GetUnitUUIDByName(c.Context(), unit.Name("!!!!!!!!!!"))
	c.Assert(err, tc.ErrorIs, unit.InvalidUnitName)
}

func (s *migrationServiceSuite) TestGetApplications(c *tc.C) {
	defer s.setupMocks(c).Finish()

	apps := []application.ExportApplication{
		{
			Name: "foo",
		},
	}

	s.state.EXPECT().GetApplicationsForExport(gomock.Any()).Return(apps, nil)

	res, err := s.service.GetApplications(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res, tc.DeepEquals, apps)
}

func (s *migrationServiceSuite) TestGetApplicationsForExportNoApplications(c *tc.C) {
	defer s.setupMocks(c).Finish()

	apps := []application.ExportApplication{}

	s.state.EXPECT().GetApplicationsForExport(gomock.Any()).Return(apps, nil)

	res, err := s.service.GetApplications(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res, tc.DeepEquals, apps)
}

func (s *migrationServiceSuite) TestGetApplicationUnits(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appID := applicationtesting.GenApplicationUUID(c)
	unitUUID := unittesting.GenUnitUUID(c)

	units := []application.ExportUnit{
		{
			UUID: unitUUID,
		},
	}

	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return(appID, nil)
	s.state.EXPECT().GetApplicationUnitsForExport(gomock.Any(), appID).Return(units, nil)

	res, err := s.service.GetApplicationUnits(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res, tc.DeepEquals, units)
}

func (s *migrationServiceSuite) TestGetApplicationUnitsNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appID := applicationtesting.GenApplicationUUID(c)

	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return(appID, applicationerrors.ApplicationNotFound)

	_, err := s.service.GetApplicationUnits(c.Context(), "foo")
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *migrationServiceSuite) TestGetApplicationUnitsNoUnits(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appID := applicationtesting.GenApplicationUUID(c)

	units := []application.ExportUnit{}

	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return(appID, nil)
	s.state.EXPECT().GetApplicationUnitsForExport(gomock.Any(), appID).Return(units, nil)

	res, err := s.service.GetApplicationUnits(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res, tc.DeepEquals, units)
}

func (s *migrationServiceSuite) TestGetApplicationUnitsInvalidName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service.GetApplicationUnits(c.Context(), "!!!!foo")
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNameNotValid)
}

func (s *migrationServiceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := s.baseSuite.setupMocks(c)

	s.service = NewMigrationService(
		s.state,
		s.clock,
		loggertesting.WrapCheckLog(c),
	)

	return ctrl
}

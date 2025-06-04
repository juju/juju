// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"testing"

	"github.com/juju/collections/set"
	"github.com/juju/description/v9"
	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	charmtesting "github.com/juju/juju/core/charm/testing"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	networktesting "github.com/juju/juju/core/network/testing"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/deployment"
	"github.com/juju/juju/internal/errors"
)

type exportApplicationSuite struct {
	exportSuite
}

func TestExportApplicationSuite(t *testing.T) {
	tc.Run(t, &exportApplicationSuite{})
}

func (s *exportApplicationSuite) TestApplicationExportEmpty(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.exportService.EXPECT().GetApplications(gomock.Any()).Return(nil, nil)

	exportOp := s.newExportOperation()

	model := description.NewModel(description.ModelArgs{
		Type: "iaas",
	})
	err := exportOp.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(model.Applications(), tc.HasLen, 0)
}

func (s *exportApplicationSuite) TestApplicationExportError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.exportService.EXPECT().GetApplications(gomock.Any()).Return(nil, errors.Errorf("boom"))

	exportOp := s.newExportOperation()

	model := description.NewModel(description.ModelArgs{
		Type: "iaas",
	})
	err := exportOp.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorMatches, ".*boom")
	c.Check(model.Applications(), tc.HasLen, 0)
}

func (s *exportApplicationSuite) TestApplicationExportNoLocator(c *tc.C) {
	defer s.setupMocks(c).Finish()

	charmUUID := charmtesting.GenCharmID(c)

	s.exportService.EXPECT().GetApplications(gomock.Any()).Return([]application.ExportApplication{{
		Name:      "prometheus",
		CharmUUID: charmUUID,
	}}, nil)

	exportOp := s.newExportOperation()

	model := description.NewModel(description.ModelArgs{
		Type: "iaas",
	})
	err := exportOp.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorMatches, `.*exporting charm URL: unsupported source ""`)
	c.Check(model.Applications(), tc.HasLen, 0)
}

func (s *exportApplicationSuite) TestApplicationExportIAASApplications(c *tc.C) {
	defer s.setupMocks(c).Finish()

	charmUUID := charmtesting.GenCharmID(c)

	s.exportService.EXPECT().GetApplications(gomock.Any()).Return([]application.ExportApplication{{
		Name:      "prometheus",
		CharmUUID: charmUUID,
		CharmLocator: charm.CharmLocator{
			Source:       charm.CharmHubSource,
			Name:         "prometheus",
			Revision:     42,
			Architecture: architecture.AMD64,
		},
	}, {
		Name:      "postgres",
		CharmUUID: charmUUID,
		CharmLocator: charm.CharmLocator{
			Source:       charm.CharmHubSource,
			Name:         "postgres",
			Revision:     42,
			Architecture: architecture.PPC64EL,
		},
	}}, nil)
	s.exportService.EXPECT().IsApplicationExposed(gomock.Any(), "prometheus").Return(false, nil)
	s.exportService.EXPECT().IsApplicationExposed(gomock.Any(), "postgres").Return(false, nil)

	s.expectCharmOriginFor("prometheus")
	s.expectApplicationConfigFor("prometheus")
	s.expectMinimalCharmFor("prometheus")
	s.expectApplicationConstraintsFor("prometheus", constraints.Value{})

	s.expectCharmOriginFor("postgres")
	s.expectApplicationConfigFor("postgres")
	s.expectMinimalCharmFor("postgres")
	s.expectApplicationConstraintsFor("postgres", constraints.Value{})

	s.expectApplicationUnitsFor("prometheus")
	s.expectApplicationUnitsFor("postgres")

	exportOp := s.newExportOperation()

	model := description.NewModel(description.ModelArgs{
		Type: "iaas",
	})
	err := exportOp.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(model.Applications(), tc.HasLen, 2)

	apps := model.Applications()
	c.Check(apps[0].Name(), tc.Equals, "prometheus")
	c.Check(apps[1].Name(), tc.Equals, "postgres")

	c.Check(apps[0].CharmURL(), tc.Equals, "ch:amd64/prometheus-42")
	c.Check(apps[1].CharmURL(), tc.Equals, "ch:ppc64el/postgres-42")

	// Check that the scaling state is not set for both applications.
	c.Check(apps[0].ProvisioningState().ScaleTarget(), tc.Equals, 0)
	c.Check(apps[0].ProvisioningState().Scaling(), tc.IsFalse)
	c.Check(apps[0].DesiredScale(), tc.Equals, 0)
	c.Check(apps[1].ProvisioningState().ScaleTarget(), tc.Equals, 0)
	c.Check(apps[1].ProvisioningState().Scaling(), tc.IsFalse)
	c.Check(apps[1].DesiredScale(), tc.Equals, 0)
}

func (s *exportApplicationSuite) TestApplicationExportCAASApplications(c *tc.C) {
	defer s.setupMocks(c).Finish()

	charmUUID := charmtesting.GenCharmID(c)

	s.exportService.EXPECT().GetApplications(gomock.Any()).Return([]application.ExportApplication{{
		Name:      "prometheus",
		CharmUUID: charmUUID,
		CharmLocator: charm.CharmLocator{
			Source:       charm.CharmHubSource,
			Name:         "prometheus",
			Revision:     42,
			Architecture: architecture.AMD64,
		},
	}, {
		Name:      "postgres",
		CharmUUID: charmUUID,
		CharmLocator: charm.CharmLocator{
			Source:       charm.CharmHubSource,
			Name:         "postgres",
			Revision:     42,
			Architecture: architecture.PPC64EL,
		},
	}}, nil)
	s.exportService.EXPECT().IsApplicationExposed(gomock.Any(), "prometheus").Return(false, nil)
	s.exportService.EXPECT().IsApplicationExposed(gomock.Any(), "postgres").Return(false, nil)

	s.expectCharmOriginFor("prometheus")
	s.expectApplicationConfigFor("prometheus")
	s.expectMinimalCharmFor("prometheus")
	s.expectApplicationConstraintsFor("prometheus", constraints.Value{})
	s.expectGetApplicationScaleStateFor("prometheus", application.ScaleState{
		Scaling:     false,
		Scale:       0,
		ScaleTarget: 0,
	})

	s.expectCharmOriginFor("postgres")
	s.expectApplicationConfigFor("postgres")
	s.expectMinimalCharmFor("postgres")
	s.expectApplicationConstraintsFor("postgres", constraints.Value{})
	s.expectGetApplicationScaleStateFor("postgres", application.ScaleState{
		Scaling:     true,
		Scale:       1,
		ScaleTarget: 2,
	})

	s.expectApplicationUnitsFor("prometheus")
	s.expectApplicationUnitsFor("postgres")

	exportOp := s.newExportOperation()

	model := description.NewModel(description.ModelArgs{
		Type: "caas",
	})
	err := exportOp.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(model.Applications(), tc.HasLen, 2)

	apps := model.Applications()
	c.Check(apps[0].Name(), tc.Equals, "prometheus")
	c.Check(apps[1].Name(), tc.Equals, "postgres")

	c.Check(apps[0].CharmURL(), tc.Equals, "ch:amd64/prometheus-42")
	c.Check(apps[1].CharmURL(), tc.Equals, "ch:ppc64el/postgres-42")

	// Check that the scaling state is set for both applications.
	c.Check(apps[0].ProvisioningState().ScaleTarget(), tc.Equals, 0)
	c.Check(apps[0].ProvisioningState().Scaling(), tc.IsFalse)
	c.Check(apps[0].DesiredScale(), tc.Equals, 0)
	c.Check(apps[1].ProvisioningState().ScaleTarget(), tc.Equals, 2)
	c.Check(apps[1].ProvisioningState().Scaling(), tc.IsTrue)
	c.Check(apps[1].DesiredScale(), tc.Equals, 1)
}

func (s *exportApplicationSuite) TestApplicationExportUnits(c *tc.C) {
	// Arrange:
	defer s.setupMocks(c).Finish()

	s.exportService.EXPECT().GetApplications(gomock.Any()).Return([]application.ExportApplication{{
		Name: "prometheus",
		CharmLocator: charm.CharmLocator{
			Source: charm.CharmHubSource,
		},
	}}, nil)
	s.exportService.EXPECT().IsApplicationExposed(gomock.Any(), "prometheus").Return(false, nil)

	s.expectCharmOriginFor("prometheus")
	s.expectApplicationConfigFor("prometheus")
	s.expectMinimalCharmFor("prometheus")
	s.expectApplicationConstraintsFor("prometheus", constraints.Value{})

	s.exportService.EXPECT().GetApplicationUnits(gomock.Any(), "prometheus").Return([]application.ExportUnit{{
		Name:      "prometheus/0",
		Machine:   "0",
		Principal: "principal1/0",
	}, {
		Name:      "prometheus/1",
		Machine:   "1",
		Principal: "principal2/0",
	}}, nil)

	// Act:
	exportOp := s.newExportOperation()
	model := description.NewModel(description.ModelArgs{
		Type: "iaas",
	})
	err := exportOp.Execute(c.Context(), model)

	// Assert:
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(model.Applications(), tc.HasLen, 1)

	apps := model.Applications()
	c.Check(apps[0].Name(), tc.Equals, "prometheus")

	// Check that the scaling state is set for the second application.
	units := apps[0].Units()
	c.Check(units, tc.HasLen, 2)
	c.Check(units[0].Name(), tc.Equals, "prometheus/0")
	c.Check(units[0].Machine(), tc.Equals, "0")
	c.Check(units[0].Principal(), tc.Equals, "principal1/0")

	c.Check(units[1].Name(), tc.Equals, "prometheus/1")
	c.Check(units[1].Machine(), tc.Equals, "1")
	c.Check(units[1].Principal(), tc.Equals, "principal2/0")
}

func (s *exportApplicationSuite) TestApplicationExportConstraints(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectApplication(c)
	s.expectMinimalCharm()
	s.expectApplicationConfig()
	s.expectApplicationUnits()
	s.exportService.EXPECT().IsApplicationExposed(gomock.Any(), "prometheus").Return(false, nil)

	cons := constraints.Value{
		AllocatePublicIP: ptr(true),
		Arch:             ptr("amd64"),
		Container:        ptr(instance.ContainerType("lxd")),
		CpuCores:         ptr(uint64(2)),
		CpuPower:         ptr(uint64(1000)),
		ImageID:          ptr("foo"),
		InstanceRole:     ptr("bar"),
		InstanceType:     ptr("baz"),
		VirtType:         ptr("vm"),
		Mem:              ptr(uint64(1024)),
		RootDisk:         ptr(uint64(1024)),
		RootDiskSource:   ptr("qux"),
		Spaces:           ptr([]string{"space0", "space1"}),
		Tags:             ptr([]string{"tag0", "tag1"}),
		Zones:            ptr([]string{"zone0", "zone1"}),
	}
	s.expectApplicationConstraints(cons)

	exportOp := s.newExportOperation()

	model := description.NewModel(description.ModelArgs{
		Type: "iaas",
	})

	err := exportOp.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(model.Applications(), tc.HasLen, 1)

	app := model.Applications()[0]
	c.Check(app.Constraints().AllocatePublicIP(), tc.Equals, true)
	c.Check(app.Constraints().Architecture(), tc.Equals, "amd64")
	c.Check(app.Constraints().Container(), tc.Equals, "lxd")
	c.Check(app.Constraints().CpuCores(), tc.Equals, uint64(2))
	c.Check(app.Constraints().CpuPower(), tc.Equals, uint64(1000))
	c.Check(app.Constraints().ImageID(), tc.Equals, "foo")
	c.Check(app.Constraints().InstanceType(), tc.Equals, "baz")
	c.Check(app.Constraints().VirtType(), tc.Equals, "vm")
	c.Check(app.Constraints().Memory(), tc.Equals, uint64(1024))
	c.Check(app.Constraints().RootDisk(), tc.Equals, uint64(1024))
	c.Check(app.Constraints().RootDiskSource(), tc.Equals, "qux")
	c.Check(app.Constraints().Spaces(), tc.DeepEquals, []string{"space0", "space1"})
	c.Check(app.Constraints().Tags(), tc.DeepEquals, []string{"tag0", "tag1"})
	c.Check(app.Constraints().Zones(), tc.DeepEquals, []string{"zone0", "zone1"})
}

func (s *exportApplicationSuite) TestExportScalingState(c *tc.C) {
	defer s.setupMocks(c).Finish()

	charmUUID := charmtesting.GenCharmID(c)

	s.exportService.EXPECT().GetApplications(gomock.Any()).Return([]application.ExportApplication{{
		Name:      "prometheus-k8s",
		CharmUUID: charmUUID,
		CharmLocator: charm.CharmLocator{
			Source:       charm.CharmHubSource,
			Name:         "prometheus-k8s",
			Revision:     42,
			Architecture: architecture.AMD64,
		},
	}}, nil)
	s.exportService.EXPECT().GetApplicationCharmOrigin(gomock.Any(), "prometheus-k8s").Return(application.CharmOrigin{
		Name:   "prometheus-k8s",
		Source: charm.CharmHubSource,
		Platform: deployment.Platform{
			OSType:       deployment.Ubuntu,
			Channel:      "24.04",
			Architecture: architecture.AMD64,
		},
	}, nil)
	s.exportService.EXPECT().IsApplicationExposed(gomock.Any(), "prometheus-k8s").Return(false, nil)

	s.expectMinimalCharmFor("prometheus-k8s")
	s.expectApplicationConfigFor("prometheus-k8s")
	s.expectApplicationConstraintsFor("prometheus-k8s", constraints.Value{})
	s.expectGetApplicationScaleStateFor("prometheus-k8s", application.ScaleState{
		Scaling:     true,
		ScaleTarget: 42,
		Scale:       1,
	})
	s.expectApplicationUnitsFor("prometheus-k8s")

	exportOp := s.newExportOperation()

	model := description.NewModel(description.ModelArgs{
		Type: "caas",
	})

	err := exportOp.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(model.Applications(), tc.HasLen, 1)

	app := model.Applications()[0]
	c.Check(app.ProvisioningState().ScaleTarget(), tc.Equals, 42)
	c.Check(app.ProvisioningState().Scaling(), tc.IsTrue)
	c.Check(app.DesiredScale(), tc.Equals, 1)
}

func (s *exportApplicationSuite) TestApplicationExportExposedEndpoints(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectApplication(c)
	s.expectMinimalCharm()
	s.expectApplicationConfig()
	s.expectApplicationUnits()
	s.expectApplicationConstraints(constraints.Value{})
	s.exportService.EXPECT().IsApplicationExposed(gomock.Any(), "prometheus").Return(true, nil)
	s.exportService.EXPECT().GetExposedEndpoints(gomock.Any(), "prometheus").Return(map[string]application.ExposedEndpoint{
		"": {
			ExposeToSpaceIDs: set.NewStrings("beta"),
		},
		"foo": {
			ExposeToSpaceIDs: set.NewStrings("space0", "space1"),
			ExposeToCIDRs:    set.NewStrings("10.0.0.0/24", "10.0.1.0/24"),
		},
	}, nil)

	exportOp := s.newExportOperation()

	model := description.NewModel(description.ModelArgs{
		Type: "iaas",
	})

	err := exportOp.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(model.Applications(), tc.HasLen, 1)

	app := model.Applications()[0]
	c.Check(app.ExposedEndpoints(), tc.HasLen, 2)
	c.Check(app.ExposedEndpoints()[""].ExposeToSpaceIDs(), tc.SameContents, []string{"beta"})
	c.Check(app.ExposedEndpoints()["foo"].ExposeToSpaceIDs(), tc.SameContents, []string{"space0", "space1"})
	c.Check(app.ExposedEndpoints()["foo"].ExposeToCIDRs(), tc.SameContents, []string{"10.0.0.0/24", "10.0.1.0/24"})
}

func (s *exportApplicationSuite) TestApplicationExportEndpointBindings(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	charmUUID := charmtesting.GenCharmID(c)
	spaceUUID := networktesting.GenSpaceUUID(c)

	s.exportService.EXPECT().GetApplications(gomock.Any()).Return([]application.ExportApplication{{
		Name:      "prometheus",
		CharmUUID: charmUUID,
		CharmLocator: charm.CharmLocator{
			Source:       charm.CharmHubSource,
			Name:         "prometheus",
			Revision:     42,
			Architecture: architecture.AMD64,
		},
		EndpointBindings: map[string]network.SpaceUUID{
			"":         network.AlphaSpaceId,
			"endpoint": spaceUUID,
			"misc":     "",
		},
	}}, nil)
	s.expectCharmOriginFor("prometheus")
	s.expectMinimalCharm()
	s.expectApplicationConfig()
	s.expectApplicationUnits()
	s.expectApplicationConstraints(constraints.Value{})
	s.exportService.EXPECT().IsApplicationExposed(gomock.Any(), "prometheus").Return(false, nil)

	// Act:
	exportOp := s.newExportOperation()
	model := description.NewModel(description.ModelArgs{
		Type: "iaas",
	})
	err := exportOp.Execute(c.Context(), model)

	// Assert:
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(model.Applications(), tc.HasLen, 1)

	app := model.Applications()[0]
	c.Check(app.EndpointBindings(), tc.HasLen, 3)
	c.Check(app.EndpointBindings()[""], tc.Equals, network.AlphaSpaceId.String())
	c.Check(app.EndpointBindings()["endpoint"], tc.Equals, spaceUUID.String())
	c.Check(app.EndpointBindings()["misc"], tc.Equals, "")
}

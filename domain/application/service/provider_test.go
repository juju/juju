// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/assumes"
	corebase "github.com/juju/juju/core/base"
	corecharm "github.com/juju/juju/core/charm"
	coreconstraints "github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/devices"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/instance"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/network"
	objectstoretesting "github.com/juju/juju/core/objectstore/testing"
	"github.com/juju/juju/core/resource"
	resourcetesting "github.com/juju/juju/core/resource/testing"
	"github.com/juju/juju/core/semversion"
	corestatus "github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/architecture"
	applicationcharm "github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/constraints"
	"github.com/juju/juju/domain/deployment"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/domain/status"
	"github.com/juju/juju/domain/storage"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/charm"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

type providerServiceSuite struct {
	baseSuite
}

func TestProviderServiceSuite(t *testing.T) {
	tc.Run(t, &providerServiceSuite{})
}

func (s *providerServiceSuite) TestCreateCAASApplication(c *tc.C) {
	defer s.setupMocks(c).Finish()

	id := coreapplication.GenID(c)
	objectStoreUUID := objectstoretesting.GenObjectStoreUUID(c)

	now := ptr(s.clock.Now())
	us := []application.AddCAASUnitArg{{
		AddUnitArg: application.AddUnitArg{
			UnitStatusArg: application.UnitStatusArg{
				AgentStatus: &status.StatusInfo[status.UnitAgentStatusType]{
					Status: status.UnitAgentStatusAllocating,
					Since:  now,
				},
				WorkloadStatus: &status.StatusInfo[status.WorkloadStatusType]{
					Status:  status.WorkloadStatusWaiting,
					Message: corestatus.MessageInstallingAgent,
					Since:   now,
				},
			},
			Constraints: constraints.Constraints{
				Arch: ptr(arch.AMD64),
			},
		},
	}}
	ch := applicationcharm.Charm{
		Metadata: applicationcharm.Metadata{
			Name:  "ubuntu",
			RunAs: "default",
			Resources: map[string]applicationcharm.Resource{
				"foo": {Name: "foo", Type: applicationcharm.ResourceTypeFile},
				"bar": {Name: "bar", Type: applicationcharm.ResourceTypeContainerImage},
				"baz": {Name: "baz", Type: applicationcharm.ResourceTypeFile},
			},
		},
		Manifest:        s.minimalManifest(),
		ReferenceName:   "ubuntu",
		Source:          applicationcharm.CharmHubSource,
		Revision:        42,
		Architecture:    architecture.ARM64,
		ObjectStoreUUID: objectStoreUUID,
	}
	platform := deployment.Platform{
		Channel:      "24.04",
		OSType:       deployment.Ubuntu,
		Architecture: architecture.ARM64,
	}

	app := application.AddCAASApplicationArg{
		BaseAddApplicationArg: application.BaseAddApplicationArg{
			Charm: ch,
			CharmDownloadInfo: &applicationcharm.DownloadInfo{
				Provenance:         applicationcharm.ProvenanceDownload,
				CharmhubIdentifier: "foo",
				DownloadURL:        "https://example.com/foo",
				DownloadSize:       42,
			},
			Platform: platform,
			Resources: []application.AddApplicationResourceArg{
				{
					Name:   "foo",
					Origin: charmresource.OriginUpload,
				},
				{
					Name:     "bar",
					Revision: ptr(42),
					Origin:   charmresource.OriginStore,
				},
				{
					Name: "baz",
					// It is ok to not have revision with origin store in case of
					// local charms
					Revision: nil,
					Origin:   charmresource.OriginStore,
				},
			},
			EndpointBindings: map[string]network.SpaceName{
				"":         "default",
				"provider": "beta",
			},
		},
		Scale: 1,
	}

	s.state.EXPECT().GetModelConstraints(gomock.Any()).Return(constraints.Constraints{}, nil)
	s.provider.EXPECT().ConstraintsValidator(gomock.Any()).Return(coreconstraints.NewValidator(), nil)
	s.state.EXPECT().GetDefaultStorageProvisioners(gomock.Any()).Return(
		application.DefaultStorageProvisioners{}, nil,
	)
	s.provider.EXPECT().PrecheckInstance(gomock.Any(), environs.PrecheckInstanceParams{
		Constraints: coreconstraints.MustParse("arch=amd64"),
		Base: corebase.Base{
			OS: "ubuntu",
			Channel: corebase.Channel{
				Track: "24.04",
			},
		},
	}).Return(nil)

	var receivedArgs []application.AddCAASUnitArg
	s.state.EXPECT().CreateCAASApplication(gomock.Any(), "ubuntu", gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, _ string, a application.AddCAASApplicationArg, args []application.AddCAASUnitArg) (coreapplication.ID, error) {
		c.Assert(a, tc.DeepEquals, app)
		receivedArgs = args
		return id, nil
	})

	s.charm.EXPECT().Actions().Return(&charm.Actions{})
	s.charm.EXPECT().Config().Return(&charm.Config{})
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
	s.charm.EXPECT().Meta().Return(&charm.Meta{
		Name: "ubuntu",
		Resources: map[string]charmresource.Meta{
			"foo": {Name: "foo", Type: charmresource.TypeFile},
			"bar": {Name: "bar", Type: charmresource.TypeContainerImage},
			"baz": {Name: "baz", Type: charmresource.TypeFile},
		},
	}).MinTimes(1)

	_, err := s.service.CreateCAASApplication(c.Context(), "ubuntu", s.charm, corecharm.Origin{
		Source:   corecharm.CharmHub,
		Platform: corecharm.MustParsePlatform("arm64/ubuntu/24.04"),
		Revision: ptr(42),
	}, AddApplicationArgs{
		ReferenceName: "ubuntu",
		DownloadInfo: &applicationcharm.DownloadInfo{
			Provenance:         applicationcharm.ProvenanceDownload,
			CharmhubIdentifier: "foo",
			DownloadURL:        "https://example.com/foo",
			DownloadSize:       42,
		},
		CharmObjectStoreUUID: objectStoreUUID,
		ResolvedResources: ResolvedResources{
			{
				Name:   "foo",
				Origin: charmresource.OriginUpload,
			},
			{
				Name:     "bar",
				Revision: ptr(42),
				Origin:   charmresource.OriginStore,
			},
			{
				Name:     "baz",
				Revision: nil,
				Origin:   charmresource.OriginStore,
			},
		},
		EndpointBindings: map[string]network.SpaceName{
			"":         "default",
			"provider": "beta",
		},
	}, AddUnitArg{})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(receivedArgs, tc.DeepEquals, us)
}

func (s *providerServiceSuite) TestCreateIAASApplicationWithApplicationStatus(c *tc.C) {
	defer s.setupMocks(c).Finish()

	id := coreapplication.GenID(c)
	objectStoreUUID := objectstoretesting.GenObjectStoreUUID(c)

	now := ptr(s.clock.Now())
	status := &status.StatusInfo[status.WorkloadStatusType]{
		Status:  status.WorkloadStatusActive,
		Message: "active",
		Data:    []byte(`{"active":true}`),
		Since:   now,
	}

	s.provider.EXPECT().ConstraintsValidator(gomock.Any()).Return(nil, nil)
	s.state.EXPECT().GetDefaultStorageProvisioners(gomock.Any()).Return(
		application.DefaultStorageProvisioners{}, nil,
	)

	var receivedArgs application.AddIAASApplicationArg
	s.state.EXPECT().CreateIAASApplication(gomock.Any(), "ubuntu", gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, _ string, appArgs application.AddIAASApplicationArg, _ []application.AddIAASUnitArg) (coreapplication.ID, []coremachine.Name, error) {
		receivedArgs = appArgs
		return id, nil, nil
	})

	s.charm.EXPECT().Actions().Return(&charm.Actions{})
	s.charm.EXPECT().Config().Return(&charm.Config{})
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
	s.charm.EXPECT().Meta().Return(&charm.Meta{
		Name: "ubuntu",
	}).MinTimes(1)

	_, err := s.service.CreateIAASApplication(c.Context(), "ubuntu", s.charm, corecharm.Origin{
		Source:   corecharm.CharmHub,
		Platform: corecharm.MustParsePlatform("arm64/ubuntu/24.04"),
		Revision: ptr(42),
	}, AddApplicationArgs{
		ReferenceName: "ubuntu",
		DownloadInfo: &applicationcharm.DownloadInfo{
			Provenance:         applicationcharm.ProvenanceDownload,
			CharmhubIdentifier: "foo",
			DownloadURL:        "https://example.com/foo",
			DownloadSize:       42,
		},
		CharmObjectStoreUUID: objectStoreUUID,
		ApplicationStatus: &corestatus.StatusInfo{
			Status:  corestatus.Active,
			Message: "active",
			Data:    map[string]interface{}{"active": true},
			Since:   now,
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(receivedArgs.Status, tc.DeepEquals, status)
}

func (s *providerServiceSuite) TestCreateIAASApplication(c *tc.C) {
	defer s.setupMocks(c).Finish()

	id := coreapplication.GenID(c)
	objectStoreUUID := objectstoretesting.GenObjectStoreUUID(c)

	ch := applicationcharm.Charm{
		Metadata: applicationcharm.Metadata{
			Name:  "ubuntu",
			RunAs: "default",
		},
		Manifest:        s.minimalManifest(),
		ReferenceName:   "ubuntu",
		Source:          applicationcharm.CharmHubSource,
		Revision:        42,
		Architecture:    architecture.ARM64,
		ObjectStoreUUID: objectStoreUUID,
	}
	platform := deployment.Platform{
		Channel:      "24.04",
		OSType:       deployment.Ubuntu,
		Architecture: architecture.ARM64,
	}

	app := application.AddIAASApplicationArg{
		BaseAddApplicationArg: application.BaseAddApplicationArg{
			Charm: ch,
			CharmDownloadInfo: &applicationcharm.DownloadInfo{
				Provenance:         applicationcharm.ProvenanceDownload,
				CharmhubIdentifier: "foo",
				DownloadURL:        "https://example.com/foo",
				DownloadSize:       42,
			},
			Platform: platform,
		},
	}

	s.state.EXPECT().GetDefaultStorageProvisioners(gomock.Any()).Return(
		application.DefaultStorageProvisioners{}, nil,
	)
	s.state.EXPECT().GetModelConstraints(gomock.Any()).Return(constraints.Constraints{}, nil)
	s.provider.EXPECT().ConstraintsValidator(gomock.Any()).Return(coreconstraints.NewValidator(), nil)
	s.provider.EXPECT().PrecheckInstance(gomock.Any(), environs.PrecheckInstanceParams{
		Constraints: coreconstraints.MustParse("cores=4 cpu-power=75 arch=amd64"),
		Base: corebase.Base{
			OS: "ubuntu",
			Channel: corebase.Channel{
				Track: "24.04",
			},
		},
		Placement: "zone=default",
	}).Return(nil)

	s.state.EXPECT().CreateIAASApplication(gomock.Any(), "ubuntu", app, gomock.Any()).Return(id, nil, nil)

	s.charm.EXPECT().Actions().Return(&charm.Actions{})
	s.charm.EXPECT().Config().Return(&charm.Config{})
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
	s.charm.EXPECT().Meta().Return(&charm.Meta{
		Name: "ubuntu",
	}).MinTimes(1)

	_, err := s.service.CreateIAASApplication(c.Context(), "ubuntu", s.charm, corecharm.Origin{
		Source:   corecharm.CharmHub,
		Platform: corecharm.MustParsePlatform("arm64/ubuntu/24.04"),
		Revision: ptr(42),
	}, AddApplicationArgs{
		ReferenceName: "ubuntu",
		DownloadInfo: &applicationcharm.DownloadInfo{
			Provenance:         applicationcharm.ProvenanceDownload,
			CharmhubIdentifier: "foo",
			DownloadURL:        "https://example.com/foo",
			DownloadSize:       42,
		},
		CharmObjectStoreUUID: objectStoreUUID,
		Constraints:          coreconstraints.MustParse("cores=4 cpu-power=75"),
	}, AddIAASUnitArg{
		AddUnitArg: AddUnitArg{
			Placement: &instance.Placement{
				Scope:     instance.ModelScope,
				Directive: "zone=default",
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *providerServiceSuite) TestCreateIAASApplicationMachineScope(c *tc.C) {
	defer s.setupMocks(c).Finish()

	id := coreapplication.GenID(c)
	objectStoreUUID := objectstoretesting.GenObjectStoreUUID(c)

	ch := applicationcharm.Charm{
		Metadata: applicationcharm.Metadata{
			Name:  "ubuntu",
			RunAs: "default",
		},
		Manifest:        s.minimalManifest(),
		ReferenceName:   "ubuntu",
		Source:          applicationcharm.CharmHubSource,
		Revision:        42,
		Architecture:    architecture.ARM64,
		ObjectStoreUUID: objectStoreUUID,
	}
	platform := deployment.Platform{
		Channel:      "24.04",
		OSType:       deployment.Ubuntu,
		Architecture: architecture.ARM64,
	}

	app := application.AddIAASApplicationArg{
		BaseAddApplicationArg: application.BaseAddApplicationArg{
			Charm: ch,
			CharmDownloadInfo: &applicationcharm.DownloadInfo{
				Provenance:         applicationcharm.ProvenanceDownload,
				CharmhubIdentifier: "foo",
				DownloadURL:        "https://example.com/foo",
				DownloadSize:       42,
			},
			Platform: platform,
		},
	}

	s.state.EXPECT().GetDefaultStorageProvisioners(gomock.Any()).Return(
		application.DefaultStorageProvisioners{}, nil,
	)
	s.state.EXPECT().GetModelConstraints(gomock.Any()).Return(constraints.Constraints{}, nil)
	s.provider.EXPECT().ConstraintsValidator(gomock.Any()).Return(coreconstraints.NewValidator(), nil)
	s.provider.EXPECT().PrecheckInstance(gomock.Any(), environs.PrecheckInstanceParams{
		Constraints: coreconstraints.MustParse("cores=4 cpu-power=75 arch=amd64"),
		Base: corebase.Base{
			OS: "ubuntu",
			Channel: corebase.Channel{
				Track: "24.04",
			},
		},
	}).Return(nil)

	s.state.EXPECT().CreateIAASApplication(gomock.Any(), "ubuntu", app, gomock.Any()).Return(id, nil, nil)

	s.charm.EXPECT().Actions().Return(&charm.Actions{})
	s.charm.EXPECT().Config().Return(&charm.Config{})
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
	s.charm.EXPECT().Meta().Return(&charm.Meta{
		Name: "ubuntu",
	}).MinTimes(1)

	_, err := s.service.CreateIAASApplication(c.Context(), "ubuntu", s.charm, corecharm.Origin{
		Source:   corecharm.CharmHub,
		Platform: corecharm.MustParsePlatform("arm64/ubuntu/24.04"),
		Revision: ptr(42),
	}, AddApplicationArgs{
		ReferenceName: "ubuntu",
		DownloadInfo: &applicationcharm.DownloadInfo{
			Provenance:         applicationcharm.ProvenanceDownload,
			CharmhubIdentifier: "foo",
			DownloadURL:        "https://example.com/foo",
			DownloadSize:       42,
		},
		CharmObjectStoreUUID: objectStoreUUID,
		Constraints:          coreconstraints.MustParse("cores=4 cpu-power=75"),
	}, AddIAASUnitArg{
		AddUnitArg: AddUnitArg{
			Placement: &instance.Placement{
				Scope:     instance.MachineScope,
				Directive: "zone=default",
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *providerServiceSuite) TestCreateIAASApplicationWithDefaultStorage(c *tc.C) {
	defer s.setupMocks(c).Finish()

	id := coreapplication.GenID(c)
	objectStoreUUID := objectstoretesting.GenObjectStoreUUID(c)

	ch := applicationcharm.Charm{
		Metadata: applicationcharm.Metadata{
			Name:  "ubuntu",
			RunAs: "default",
			Storage: map[string]applicationcharm.Storage{
				"foo-data": {
					Name:        "foo-data",
					Type:        applicationcharm.StorageBlock,
					CountMin:    2,
					CountMax:    -1,
					MinimumSize: 2048,
				},
				"bar-data": {
					Name:        "bar-data",
					Type:        applicationcharm.StorageFilesystem,
					CountMin:    1,
					CountMax:    1,
					MinimumSize: 4096,
				},
			},
		},
		Manifest:        s.minimalManifest(),
		ReferenceName:   "ubuntu",
		Source:          applicationcharm.CharmHubSource,
		Revision:        42,
		Architecture:    architecture.ARM64,
		ObjectStoreUUID: objectStoreUUID,
	}
	platform := deployment.Platform{
		Channel:      "24.04",
		OSType:       deployment.Ubuntu,
		Architecture: architecture.ARM64,
	}

	app := application.AddIAASApplicationArg{
		BaseAddApplicationArg: application.BaseAddApplicationArg{
			Charm: ch,
			CharmDownloadInfo: &applicationcharm.DownloadInfo{
				Provenance:         applicationcharm.ProvenanceDownload,
				CharmhubIdentifier: "foo",
				DownloadURL:        "https://example.com/foo",
				DownloadSize:       42,
			},
			Platform: platform,
			StorageDirectives: []application.CreateApplicationStorageDirectiveArg{{
				Name:         "foo-data",
				Count:        2,
				ProviderType: ptr("a-blockdevice-provider"),
				Size:         2048,
			}, {
				Name:         "bar-data",
				Count:        1,
				ProviderType: ptr("a-filesystem-provider"),
				Size:         4096,
			}},
		},
	}

	s.state.EXPECT().GetDefaultStorageProvisioners(gomock.Any()).Return(
		application.DefaultStorageProvisioners{
			BlockdeviceProviderType: ptr("a-blockdevice-provider"),
			FilesystemProviderType:  ptr("a-filesystem-provider"),
		}, nil,
	)
	s.state.EXPECT().GetModelConstraints(gomock.Any()).Return(constraints.Constraints{}, nil)
	s.provider.EXPECT().ConstraintsValidator(gomock.Any()).Return(coreconstraints.NewValidator(), nil)
	s.provider.EXPECT().PrecheckInstance(gomock.Any(), environs.PrecheckInstanceParams{
		Constraints: coreconstraints.MustParse("cores=4 cpu-power=75 arch=amd64"),
		Base: corebase.Base{
			OS: "ubuntu",
			Channel: corebase.Channel{
				Track: "24.04",
			},
		},
		Placement: "zone=default",
	}).Return(nil)

	s.state.EXPECT().CreateIAASApplication(gomock.Any(), "ubuntu", gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, _ string, a application.AddIAASApplicationArg, _ []application.AddIAASUnitArg) (coreapplication.ID, []coremachine.Name, error) {
		mc := tc.NewMultiChecker()
		mc.AddExpr(
			`_.BaseAddApplicationArg.StorageDirectives`,
			tc.UnorderedMatch[[]application.CreateApplicationStorageDirectiveArg](tc.DeepEquals),
			tc.ExpectedValue,
		)
		c.Assert(a, mc, app)
		return id, nil, nil
	})

	s.charm.EXPECT().Actions().Return(&charm.Actions{})
	s.charm.EXPECT().Config().Return(&charm.Config{})
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
	s.charm.EXPECT().Meta().Return(&charm.Meta{
		Name: "ubuntu",
		Storage: map[string]charm.Storage{
			"foo-data": {
				Name:        "foo-data",
				Type:        charm.StorageBlock,
				CountMin:    2,
				CountMax:    -1,
				MinimumSize: 2048,
			},
			"bar-data": {
				Name:        "bar-data",
				Type:        charm.StorageFilesystem,
				CountMin:    1,
				CountMax:    1,
				MinimumSize: 4096,
			},
		},
	}).MinTimes(1)

	_, err := s.service.CreateIAASApplication(c.Context(), "ubuntu", s.charm, corecharm.Origin{
		Source:   corecharm.CharmHub,
		Platform: corecharm.MustParsePlatform("arm64/ubuntu/24.04"),
		Revision: ptr(42),
	}, AddApplicationArgs{
		ReferenceName: "ubuntu",
		DownloadInfo: &applicationcharm.DownloadInfo{
			Provenance:         applicationcharm.ProvenanceDownload,
			CharmhubIdentifier: "foo",
			DownloadURL:        "https://example.com/foo",
			DownloadSize:       42,
		},
		CharmObjectStoreUUID: objectStoreUUID,
		Constraints:          coreconstraints.MustParse("cores=4 cpu-power=75"),
	}, AddIAASUnitArg{
		AddUnitArg: AddUnitArg{
			Placement: &instance.Placement{
				Scope:     instance.ModelScope,
				Directive: "zone=default",
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *providerServiceSuite) TestCreateIAASApplicationWithExplicitStorage(c *tc.C) {
	defer s.setupMocks(c).Finish()

	id := coreapplication.GenID(c)
	objectStoreUUID := objectstoretesting.GenObjectStoreUUID(c)
	blockDeviceStoragePoolUUID := storage.StoragePoolUUID(uuid.MustNewUUID().String())
	filesystemStoragePoolUUID := storage.StoragePoolUUID(uuid.MustNewUUID().String())

	ch := applicationcharm.Charm{
		Metadata: applicationcharm.Metadata{
			Name:  "ubuntu",
			RunAs: "default",
			Storage: map[string]applicationcharm.Storage{
				"foo-data": {
					Name:        "foo-data",
					Type:        applicationcharm.StorageBlock,
					CountMin:    2,
					CountMax:    -1,
					MinimumSize: 2048,
				},
				"bar-data": {
					Name:        "bar-data",
					Type:        applicationcharm.StorageFilesystem,
					CountMin:    1,
					CountMax:    1,
					MinimumSize: 4096,
				},
			},
		},
		Manifest:        s.minimalManifest(),
		ReferenceName:   "ubuntu",
		Source:          applicationcharm.CharmHubSource,
		Revision:        42,
		Architecture:    architecture.ARM64,
		ObjectStoreUUID: objectStoreUUID,
	}
	platform := deployment.Platform{
		Channel:      "24.04",
		OSType:       deployment.Ubuntu,
		Architecture: architecture.ARM64,
	}

	app := application.AddIAASApplicationArg{
		BaseAddApplicationArg: application.BaseAddApplicationArg{
			Charm: ch,
			CharmDownloadInfo: &applicationcharm.DownloadInfo{
				Provenance:         applicationcharm.ProvenanceDownload,
				CharmhubIdentifier: "foo",
				DownloadURL:        "https://example.com/foo",
				DownloadSize:       42,
			},
			Platform: platform,
			StorageDirectives: []application.CreateApplicationStorageDirectiveArg{{
				Name:     "foo-data",
				Count:    2,
				PoolUUID: &blockDeviceStoragePoolUUID,
				Size:     2048,
			}, {
				Name:     "bar-data",
				Count:    1,
				PoolUUID: &filesystemStoragePoolUUID,
				Size:     4096,
			}},
		},
	}

	s.state.EXPECT().GetDefaultStorageProvisioners(gomock.Any()).Return(
		application.DefaultStorageProvisioners{
			BlockdeviceProviderType: ptr("unwanted-blockdevice-provider"),
			FilesystemPoolUUID:      &filesystemStoragePoolUUID,
		}, nil,
	)
	s.state.EXPECT().GetModelConstraints(gomock.Any()).Return(constraints.Constraints{}, nil)
	s.provider.EXPECT().ConstraintsValidator(gomock.Any()).Return(coreconstraints.NewValidator(), nil)
	s.provider.EXPECT().PrecheckInstance(gomock.Any(), environs.PrecheckInstanceParams{
		Constraints: coreconstraints.MustParse("cores=4 cpu-power=75 arch=amd64"),
		Base: corebase.Base{
			OS: "ubuntu",
			Channel: corebase.Channel{
				Track: "24.04",
			},
		},
		Placement: "zone=default",
	}).Return(nil)

	s.state.EXPECT().CreateIAASApplication(gomock.Any(), "ubuntu", gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, _ string, a application.AddIAASApplicationArg, _ []application.AddIAASUnitArg) (coreapplication.ID, []coremachine.Name, error) {
		mc := tc.NewMultiChecker()
		mc.AddExpr(
			`_.BaseAddApplicationArg.StorageDirectives`,
			tc.UnorderedMatch[[]application.CreateApplicationStorageDirectiveArg](tc.DeepEquals),
			tc.ExpectedValue,
		)
		c.Assert(a, mc, app)
		return id, nil, nil
	})

	s.charm.EXPECT().Actions().Return(&charm.Actions{})
	s.charm.EXPECT().Config().Return(&charm.Config{})
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
	s.charm.EXPECT().Meta().Return(&charm.Meta{
		Name: "ubuntu",
		Storage: map[string]charm.Storage{
			"foo-data": {
				Name:        "foo-data",
				Type:        charm.StorageBlock,
				CountMin:    2,
				CountMax:    -1,
				MinimumSize: 2048,
			},
			"bar-data": {
				Name:        "bar-data",
				Type:        charm.StorageFilesystem,
				CountMin:    1,
				CountMax:    1,
				MinimumSize: 4096,
			},
		},
	}).MinTimes(1)

	s.storageValidator.EXPECT().CheckPoolSupportsCharmStorage(
		gomock.Any(), blockDeviceStoragePoolUUID, charm.StorageBlock).Return(true, nil)
	s.storageValidator.EXPECT().CheckProviderTypeSupportsCharmStorage(
		gomock.Any(), "special-filesystem-provider", charm.StorageFilesystem).Return(true, nil)

	_, err := s.service.CreateIAASApplication(c.Context(), "ubuntu", s.charm, corecharm.Origin{
		Source:   corecharm.CharmHub,
		Platform: corecharm.MustParsePlatform("arm64/ubuntu/24.04"),
		Revision: ptr(42),
	}, AddApplicationArgs{
		ReferenceName: "ubuntu",
		DownloadInfo: &applicationcharm.DownloadInfo{
			Provenance:         applicationcharm.ProvenanceDownload,
			CharmhubIdentifier: "foo",
			DownloadURL:        "https://example.com/foo",
			DownloadSize:       42,
		},
		CharmObjectStoreUUID: objectStoreUUID,
		Constraints:          coreconstraints.MustParse("cores=4 cpu-power=75"),
		StorageDirectiveOverrides: map[string]ApplicationStorageDirectiveOverride{
			"foo-data": {
				PoolUUID: &blockDeviceStoragePoolUUID,
			},
			"bar-data": {
				ProviderType: ptr("special-filesystem-provider"),
			},
		},
	}, AddIAASUnitArg{
		AddUnitArg: AddUnitArg{
			Placement: &instance.Placement{
				Scope:     instance.ModelScope,
				Directive: "zone=default",
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *providerServiceSuite) TestCreateIAASApplicationPrecheckFailure(c *tc.C) {
	defer s.setupMocks(c).Finish()

	objectStoreUUID := objectstoretesting.GenObjectStoreUUID(c)

	s.state.EXPECT().GetDefaultStorageProvisioners(gomock.Any()).Return(
		application.DefaultStorageProvisioners{
			FilesystemProviderType: ptr("fast"),
		}, nil,
	)
	s.state.EXPECT().GetModelConstraints(gomock.Any()).Return(constraints.Constraints{}, nil)
	s.provider.EXPECT().ConstraintsValidator(gomock.Any()).Return(coreconstraints.NewValidator(), nil)
	s.provider.EXPECT().PrecheckInstance(gomock.Any(), environs.PrecheckInstanceParams{
		Constraints: coreconstraints.MustParse("cores=4 cpu-power=75 arch=amd64"),
		Base: corebase.Base{
			OS: "ubuntu",
			Channel: corebase.Channel{
				Track: "24.04",
			},
		},
	}).Return(errors.Errorf("boom"))

	s.charm.EXPECT().Actions().Return(&charm.Actions{})
	s.charm.EXPECT().Config().Return(&charm.Config{})
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
	s.charm.EXPECT().Meta().Return(&charm.Meta{
		Name: "ubuntu",
	}).MinTimes(1)

	_, err := s.service.CreateIAASApplication(c.Context(), "ubuntu", s.charm, corecharm.Origin{
		Source:   corecharm.CharmHub,
		Platform: corecharm.MustParsePlatform("arm64/ubuntu/24.04"),
		Revision: ptr(42),
	}, AddApplicationArgs{
		ReferenceName: "ubuntu",
		DownloadInfo: &applicationcharm.DownloadInfo{
			Provenance:         applicationcharm.ProvenanceDownload,
			CharmhubIdentifier: "foo",
			DownloadURL:        "https://example.com/foo",
			DownloadSize:       42,
		},
		CharmObjectStoreUUID: objectStoreUUID,
		Constraints:          coreconstraints.MustParse("cores=4 cpu-power=75"),
	}, AddIAASUnitArg{
		AddUnitArg: AddUnitArg{
			Placement: &instance.Placement{
				Scope:     instance.MachineScope,
				Directive: "zone=default",
			},
		},
	})
	c.Assert(err, tc.ErrorMatches, `.*boom`)
}

func (s *providerServiceSuite) TestCreateIAASApplicationPendingResources(c *tc.C) {
	defer s.setupMocks(c).Finish()

	id := coreapplication.GenID(c)
	objectStoreUUID := objectstoretesting.GenObjectStoreUUID(c)

	ch := applicationcharm.Charm{
		Metadata: applicationcharm.Metadata{
			Name:  "ubuntu",
			RunAs: "default",
			Resources: map[string]applicationcharm.Resource{
				"foo": {Name: "foo", Type: applicationcharm.ResourceTypeFile},
			},
		},
		Manifest:        s.minimalManifest(),
		ReferenceName:   "ubuntu",
		Source:          applicationcharm.CharmHubSource,
		Revision:        42,
		Architecture:    architecture.ARM64,
		ObjectStoreUUID: objectStoreUUID,
	}
	platform := deployment.Platform{
		Channel:      "24.04",
		OSType:       deployment.Ubuntu,
		Architecture: architecture.ARM64,
	}

	resourceUUID := resourcetesting.GenResourceUUID(c)
	app := application.AddIAASApplicationArg{
		BaseAddApplicationArg: application.BaseAddApplicationArg{
			Charm: ch,
			CharmDownloadInfo: &applicationcharm.DownloadInfo{
				Provenance:         applicationcharm.ProvenanceDownload,
				CharmhubIdentifier: "foo",
				DownloadURL:        "https://example.com/foo",
				DownloadSize:       42,
			},
			Platform:         platform,
			PendingResources: []resource.UUID{resourceUUID},
		},
	}

	s.state.EXPECT().GetDefaultStorageProvisioners(gomock.Any()).Return(
		application.DefaultStorageProvisioners{
			FilesystemProviderType: ptr("fast"),
		}, nil,
	)
	s.state.EXPECT().GetModelConstraints(gomock.Any()).Return(constraints.Constraints{}, nil)
	s.provider.EXPECT().ConstraintsValidator(gomock.Any()).Return(coreconstraints.NewValidator(), nil)
	s.provider.EXPECT().PrecheckInstance(gomock.Any(), environs.PrecheckInstanceParams{
		Constraints: coreconstraints.MustParse("cores=4 cpu-power=75 arch=amd64"),
		Base: corebase.Base{
			OS: "ubuntu",
			Channel: corebase.Channel{
				Track: "24.04",
			},
		},
	}).Return(nil)

	s.state.EXPECT().CreateIAASApplication(gomock.Any(), "ubuntu", gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, _ string, a application.AddIAASApplicationArg, _ []application.AddIAASUnitArg) (coreapplication.ID, []coremachine.Name, error) {
		c.Assert(a, tc.DeepEquals, app)
		return id, nil, nil
	})

	s.charm.EXPECT().Actions().Return(&charm.Actions{})
	s.charm.EXPECT().Config().Return(&charm.Config{})
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
	s.charm.EXPECT().Meta().Return(&charm.Meta{
		Name: "ubuntu",
		Resources: map[string]charmresource.Meta{
			"foo": {Name: "foo", Type: charmresource.TypeFile},
		},
	}).MinTimes(1)

	_, err := s.service.CreateIAASApplication(c.Context(), "ubuntu", s.charm, corecharm.Origin{
		Source:   corecharm.CharmHub,
		Platform: corecharm.MustParsePlatform("arm64/ubuntu/24.04"),
		Revision: ptr(42),
	}, AddApplicationArgs{
		ReferenceName: "ubuntu",
		DownloadInfo: &applicationcharm.DownloadInfo{
			Provenance:         applicationcharm.ProvenanceDownload,
			CharmhubIdentifier: "foo",
			DownloadURL:        "https://example.com/foo",
			DownloadSize:       42,
		},
		CharmObjectStoreUUID: objectStoreUUID,
		PendingResources:     []resource.UUID{resourceUUID},
		Constraints:          coreconstraints.MustParse("cores=4 cpu-power=75"),
	}, AddIAASUnitArg{})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *providerServiceSuite) TestCreateIAASApplicationWithInvalidApplicationName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service.CreateIAASApplication(c.Context(), "666", s.charm, corecharm.Origin{
		Source:   corecharm.CharmHub,
		Platform: corecharm.MustParsePlatform("arm64/ubuntu/24.04"),
		Revision: ptr(42),
	}, AddApplicationArgs{
		ReferenceName: "ubuntu",
	})
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNameNotValid)
}

func (s *providerServiceSuite) TestCreateIAASApplicationWithInvalidCharmName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.charm.EXPECT().Meta().Return(&charm.Meta{
		Name: "666",
	}).AnyTimes()

	_, err := s.service.CreateIAASApplication(c.Context(), "ubuntu", s.charm, corecharm.Origin{
		Source:   corecharm.CharmHub,
		Platform: corecharm.MustParsePlatform("arm64/ubuntu/24.04"),
		Revision: ptr(42),
	}, AddApplicationArgs{
		ReferenceName: "ubuntu",
	})
	c.Assert(err, tc.ErrorIs, applicationerrors.CharmNameNotValid)
}

func (s *providerServiceSuite) TestCreateIAASApplicationWithInvalidReferenceName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.charm.EXPECT().Meta().Return(&charm.Meta{
		Name: "ubuntu",
	}).AnyTimes()
	s.charm.EXPECT().Manifest().Return(&charm.Manifest{
		Bases: []charm.Base{{}},
	}).AnyTimes()

	_, err := s.service.CreateIAASApplication(c.Context(), "ubuntu", s.charm, corecharm.Origin{
		Source:   corecharm.CharmHub,
		Platform: corecharm.MustParsePlatform("arm64/ubuntu/24.04"),
		Revision: ptr(42),
	}, AddApplicationArgs{
		ReferenceName: "666",
		DownloadInfo: &applicationcharm.DownloadInfo{
			CharmhubIdentifier: "foo",
			DownloadURL:        "https://example.com/foo",
			DownloadSize:       42,
		},
	})
	c.Assert(err, tc.ErrorIs, applicationerrors.CharmNameNotValid)
}

func (s *providerServiceSuite) TestCreateIAASApplicationWithNoCharmName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.charm.EXPECT().Meta().Return(&charm.Meta{}).AnyTimes()

	_, err := s.service.CreateIAASApplication(c.Context(), "foo", s.charm, corecharm.Origin{
		Platform: corecharm.MustParsePlatform("arm64/ubuntu/24.04"),
	}, AddApplicationArgs{})
	c.Assert(err, tc.ErrorIs, applicationerrors.CharmNameNotValid)
}

func (s *providerServiceSuite) TestCreateIAASApplicationWithNoApplicationOrCharmName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.charm.EXPECT().Meta().Return(&charm.Meta{}).AnyTimes()

	_, err := s.service.CreateIAASApplication(c.Context(), "", s.charm, corecharm.Origin{
		Platform: corecharm.MustParsePlatform("arm64/ubuntu/24.04"),
	}, AddApplicationArgs{})
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNameNotValid)
}

func (s *providerServiceSuite) TestCreateIAASApplicationWithNoMeta(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.charm.EXPECT().Meta().Return(nil).MinTimes(1)

	_, err := s.service.CreateIAASApplication(c.Context(), "foo", s.charm, corecharm.Origin{
		Platform: corecharm.MustParsePlatform("arm64/ubuntu/24.04"),
	}, AddApplicationArgs{})
	c.Assert(err, tc.ErrorIs, applicationerrors.CharmMetadataNotValid)
}

func (s *providerServiceSuite) TestCreateIAASApplicationWithNoArchitecture(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.charm.EXPECT().Meta().Return(&charm.Meta{Name: "foo"}).MinTimes(1)
	s.charm.EXPECT().Manifest().Return(&charm.Manifest{
		Bases: []charm.Base{{}},
	}).MinTimes(1)

	_, err := s.service.CreateIAASApplication(c.Context(), "foo", s.charm, corecharm.Origin{
		Source:   corecharm.CharmHub,
		Platform: corecharm.Platform{Channel: "24.04", OS: "ubuntu"},
	}, AddApplicationArgs{
		ReferenceName: "foo",
		DownloadInfo: &applicationcharm.DownloadInfo{
			Provenance:         applicationcharm.ProvenanceDownload,
			CharmhubIdentifier: "foo",
			DownloadURL:        "https://example.com/foo",
			DownloadSize:       42,
		},
	})
	c.Assert(err, tc.ErrorIs, applicationerrors.CharmOriginNotValid)
}

func (s *providerServiceSuite) TestCreateApplicationWithInvalidResourcesNotAllResourcesResolved(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.charm.EXPECT().Meta().Return(&charm.Meta{Name: "foo", Resources: map[string]charmresource.Meta{
		"not-resolved": {Name: "not-resolved"},
	}}).MinTimes(1)
	s.charm.EXPECT().Manifest().Return(&charm.Manifest{
		Bases: []charm.Base{{
			Name: "ubuntu",
			Channel: charm.Channel{
				Risk: charm.Stable,
			},
			Architectures: []string{"amd64"},
		}},
	}).MinTimes(1)

	_, err := s.service.CreateIAASApplication(c.Context(), "foo", s.charm, corecharm.Origin{
		Source:   corecharm.Local,
		Platform: corecharm.MustParsePlatform("arm64/ubuntu/24.04"),
	},
		AddApplicationArgs{
			ReferenceName:     "foo",
			ResolvedResources: nil,
		})
	c.Assert(err, tc.ErrorIs, applicationerrors.InvalidResourceArgs)
	c.Assert(err, tc.ErrorMatches,
		".*create application: charm has resources which have not provided: invalid resource args")
}

// TestCreateApplicationWithInvalidResourceBothTypes tests that resolved resources and
// pending resources are mutually exclusive.
func (s *providerServiceSuite) TestCreateIAASApplicationWithInvalidResourceBothTypes(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.charm.EXPECT().Meta().Return(&charm.Meta{Name: "foo", Resources: map[string]charmresource.Meta{
		"not-resolved": {Name: "not-resolved"},
	}}).MinTimes(1)
	s.charm.EXPECT().Manifest().Return(&charm.Manifest{
		Bases: []charm.Base{{
			Name: "ubuntu",
			Channel: charm.Channel{
				Risk: charm.Stable,
			},
			Architectures: []string{"amd64"},
		}},
	}).MinTimes(1)

	_, err := s.service.CreateIAASApplication(c.Context(), "foo", s.charm,
		corecharm.Origin{
			Source:   corecharm.Local,
			Platform: corecharm.MustParsePlatform("arm64/ubuntu/24.04"),
		},
		AddApplicationArgs{
			ReferenceName:     "foo",
			ResolvedResources: ResolvedResources{ResolvedResource{Name: "testme"}},
			PendingResources:  []resource.UUID{resourcetesting.GenResourceUUID(c)},
		})
	c.Assert(err, tc.ErrorIs, applicationerrors.InvalidResourceArgs)
	// There are many places where InvalidResourceArgs are returned,
	// verify we have the expected one.
	c.Assert(err, tc.ErrorMatches,
		".*create application: cannot have both pending and resolved resources: invalid resource args")
}

func (s *providerServiceSuite) TestCreateIAASApplicationWithInvalidResourcesMoreResolvedThanCharmResources(c *tc.C) {
	resources := ResolvedResources{
		{
			Name:     "not-in-charm",
			Origin:   charmresource.OriginStore,
			Revision: ptr(42),
		},
	}
	s.testCreateIAASApplicationWithInvalidResource(c, resources)
}

func (s *providerServiceSuite) TestCreateIAASApplicationWithInvalidResourcesUploadWithRevision(c *tc.C) {
	resources := ResolvedResources{
		{
			Name:     "Upload-revision",
			Origin:   charmresource.OriginUpload,
			Revision: ptr(42),
		},
	}
	s.testCreateIAASApplicationWithInvalidResource(c, resources)
}

func (s *providerServiceSuite) TestCreateIAASApplicationWithInvalidResourcesNoName(c *tc.C) {
	resources := ResolvedResources{
		{
			Origin:   charmresource.OriginStore,
			Revision: ptr(42),
		},
	}
	s.testCreateIAASApplicationWithInvalidResource(c, resources)
}

func (s *providerServiceSuite) TestCreateIAASApplicationWithInvalidResourcesInvalidOrigin(c *tc.C) {
	resources := ResolvedResources{
		{
			Name:   "invalid-origin",
			Origin: 42,
		},
	}
	s.testCreateIAASApplicationWithInvalidResource(c, resources)
}

func (s *providerServiceSuite) testCreateIAASApplicationWithInvalidResource(c *tc.C, resources ResolvedResources) {
	defer s.setupMocks(c).Finish()

	s.charm.EXPECT().Meta().Return(&charm.Meta{Name: "foo"}).MinTimes(1)
	s.charm.EXPECT().Manifest().Return(&charm.Manifest{
		Bases: []charm.Base{{
			Name: "ubuntu",
			Channel: charm.Channel{
				Risk: charm.Stable,
			},
			Architectures: []string{"amd64"},
		}},
	}).MinTimes(1)

	_, err := s.service.CreateIAASApplication(c.Context(), "foo", s.charm, corecharm.Origin{
		Source:   corecharm.Local,
		Platform: corecharm.MustParsePlatform("arm64/ubuntu/24.04"),
	},
		AddApplicationArgs{
			ReferenceName:     "foo",
			ResolvedResources: resources,
		})
	c.Assert(err, tc.ErrorIs, applicationerrors.InvalidResourceArgs)
}

func (s *providerServiceSuite) TestCreateIAASApplicationError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	id := coreapplication.GenID(c)

	s.provider.EXPECT().ConstraintsValidator(gomock.Any()).Return(nil, nil)

	rErr := errors.New("boom")
	s.state.EXPECT().GetDefaultStorageProvisioners(gomock.Any()).Return(
		application.DefaultStorageProvisioners{}, nil,
	)
	s.state.EXPECT().CreateIAASApplication(gomock.Any(), "foo", gomock.Any(), []application.AddIAASUnitArg{}).Return(id, nil, rErr)

	s.charm.EXPECT().Meta().Return(&charm.Meta{
		Name: "foo",
	}).MinTimes(1)
	s.charm.EXPECT().Manifest().Return(&charm.Manifest{Bases: []charm.Base{{
		Name:          "ubuntu",
		Channel:       charm.Channel{Risk: charm.Beta},
		Architectures: []string{"arm64"},
	}}}).MinTimes(1)
	s.charm.EXPECT().Actions().Return(&charm.Actions{})
	s.charm.EXPECT().Config().Return(&charm.Config{})

	_, err := s.service.CreateIAASApplication(c.Context(), "foo", s.charm, corecharm.Origin{
		Source:   corecharm.CharmHub,
		Platform: corecharm.MustParsePlatform("arm64/ubuntu/24.04"),
	}, AddApplicationArgs{
		ReferenceName: "foo",
		DownloadInfo: &applicationcharm.DownloadInfo{
			Provenance:         applicationcharm.ProvenanceDownload,
			CharmhubIdentifier: "foo",
			DownloadURL:        "https://example.com/foo",
			DownloadSize:       42,
		},
	})
	c.Check(err, tc.ErrorIs, rErr)
	c.Assert(err, tc.ErrorMatches, `creating IAAS application "foo": boom`)
}

// TODO (tlm): Need to add the expectation that the default storage source is
// added to the directives.
func (s *providerServiceSuite) TestCreateIAASApplicationWithStorageBlock(c *tc.C) {
	defer s.setupMocks(c).Finish()

	id := coreapplication.GenID(c)

	now := ptr(s.clock.Now())
	us := []application.AddIAASUnitArg{{
		AddUnitArg: application.AddUnitArg{
			UnitStatusArg: application.UnitStatusArg{
				AgentStatus: &status.StatusInfo[status.UnitAgentStatusType]{
					Status: status.UnitAgentStatusAllocating,
					Since:  now,
				},
				WorkloadStatus: &status.StatusInfo[status.WorkloadStatusType]{
					Status:  status.WorkloadStatusWaiting,
					Message: "waiting for machine",
					Since:   now,
				},
			},
			CreateUnitStorageArg: application.CreateUnitStorageArg{
				StorageDirectives: []application.CreateUnitStorageDirectiveArg{
					{
						Count:        1,
						Name:         "data",
						ProviderType: ptr("rootfs"),
						Size:         10,
					},
				},
				StorageInstances: []application.CreateUnitStorageInstanceArg{
					{
						Name: "data",
					},
				},
				StorageToAttach: []storage.StorageInstanceUUID{""},
				StorageToOwn:    []storage.StorageInstanceUUID{""},
			},
		},
		Platform: deployment.Platform{
			OSType:  deployment.Ubuntu,
			Channel: "24.04",
		},
	}}
	ch := applicationcharm.Charm{
		Metadata: applicationcharm.Metadata{
			Name:  "foo",
			RunAs: "default",
			Storage: map[string]applicationcharm.Storage{
				"data": {
					Name:        "data",
					Type:        applicationcharm.StorageBlock,
					Shared:      false,
					CountMin:    1,
					CountMax:    2,
					MinimumSize: 10,
				},
			},
		},
		Manifest:      s.minimalManifest(),
		ReferenceName: "foo",
		Source:        applicationcharm.LocalSource,
		Revision:      42,
		Architecture:  architecture.AMD64,
	}
	platform := deployment.Platform{
		Channel:      "24.04",
		OSType:       deployment.Ubuntu,
		Architecture: architecture.AMD64,
	}
	app := application.AddIAASApplicationArg{
		BaseAddApplicationArg: application.BaseAddApplicationArg{
			Charm: ch,
			CharmDownloadInfo: &applicationcharm.DownloadInfo{
				CharmhubIdentifier: "foo",
				DownloadURL:        "https://example.com/foo",
				DownloadSize:       42,
			},
			Platform: platform,
			StorageDirectives: []application.CreateApplicationStorageDirectiveArg{
				{
					Name:         "data",
					Count:        1,
					Size:         10,
					ProviderType: ptr("rootfs"),
				},
			},
		},
	}

	s.provider.EXPECT().ConstraintsValidator(gomock.Any()).Return(nil, nil)
	s.provider.EXPECT().PrecheckInstance(gomock.Any(), environs.PrecheckInstanceParams{
		Base: corebase.Base{
			OS: "ubuntu",
			Channel: corebase.Channel{
				Track: "24.04",
			},
		},
	}).Return(nil)

	s.state.EXPECT().GetDefaultStorageProvisioners(gomock.Any()).Return(
		application.DefaultStorageProvisioners{
			BlockdeviceProviderType: ptr("rootfs"),
		}, nil,
	)
	s.state.EXPECT().CreateIAASApplication(gomock.Any(), "foo", gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, n string, a application.AddIAASApplicationArg, u []application.AddIAASUnitArg) (coreapplication.ID, []coremachine.Name, error) {
		c.Assert(a, tc.DeepEquals, app)
		mc := tc.NewMultiChecker()
		mc.AddExpr(`_[_].AddUnitArg.CreateUnitStorageArg`,
			s.createUnitStorageArgChecker(), tc.ExpectedValue)
		c.Assert(u, mc, us)
		return id, nil, nil
	})

	s.charm.EXPECT().Actions().Return(&charm.Actions{})
	s.charm.EXPECT().Config().Return(&charm.Config{})
	s.charm.EXPECT().Meta().Return(&charm.Meta{
		Name: "foo",
		Storage: map[string]charm.Storage{
			"data": {
				Name:        "data",
				Type:        charm.StorageBlock,
				Shared:      false,
				CountMin:    1,
				CountMax:    2,
				MinimumSize: 10,
			},
		},
	}).MinTimes(1)
	s.charm.EXPECT().Manifest().Return(&charm.Manifest{Bases: []charm.Base{{
		Name:          "ubuntu",
		Channel:       charm.Channel{Risk: charm.Stable},
		Architectures: []string{"amd64"},
	}}}).MinTimes(1)

	_, err := s.service.CreateIAASApplication(c.Context(), "foo", s.charm, corecharm.Origin{
		Source:   corecharm.Local,
		Platform: corecharm.MustParsePlatform("amd64/ubuntu/24.04"),
		Revision: ptr(42),
	}, AddApplicationArgs{
		ReferenceName: "foo",
		DownloadInfo: &applicationcharm.DownloadInfo{
			CharmhubIdentifier: "foo",
			DownloadURL:        "https://example.com/foo",
			DownloadSize:       42,
		},
	}, AddIAASUnitArg{})
	c.Assert(err, tc.ErrorIsNil)
}

// TODO (tlm): Add a case where a default block and or file system source cannot
// be supplied. What happens then?
func (s *providerServiceSuite) TestCreateIAASApplicationWithStorageBlockDefaultSource(c *tc.C) {
	defer s.setupMocks(c).Finish()

	id := coreapplication.GenID(c)

	now := ptr(s.clock.Now())
	us := []application.AddIAASUnitArg{{
		AddUnitArg: application.AddUnitArg{
			UnitStatusArg: application.UnitStatusArg{
				AgentStatus: &status.StatusInfo[status.UnitAgentStatusType]{
					Status: status.UnitAgentStatusAllocating,
					Since:  now,
				},
				WorkloadStatus: &status.StatusInfo[status.WorkloadStatusType]{
					Status:  status.WorkloadStatusWaiting,
					Message: corestatus.MessageWaitForMachine,
					Since:   now,
				},
			},
			CreateUnitStorageArg: application.CreateUnitStorageArg{
				StorageDirectives: []application.CreateUnitStorageDirectiveArg{
					{
						Name:         "data",
						Count:        3,
						Size:         10,
						ProviderType: ptr("fast"),
					},
				},
				StorageInstances: []application.CreateUnitStorageInstanceArg{
					{
						Name: "data",
					},
					{
						Name: "data",
					},
					{
						Name: "data",
					},
				},
				StorageToAttach: []storage.StorageInstanceUUID{"", "", ""},
				StorageToOwn:    []storage.StorageInstanceUUID{"", "", ""},
			},
		},
		Platform: deployment.Platform{
			OSType:  deployment.Ubuntu,
			Channel: "24.04",
		},
	}}
	ch := applicationcharm.Charm{
		Metadata: applicationcharm.Metadata{
			Name:  "foo",
			RunAs: "default",
			Storage: map[string]applicationcharm.Storage{
				"data": {
					Name:        "data",
					Type:        applicationcharm.StorageBlock,
					Shared:      false,
					CountMin:    1,
					CountMax:    3,
					MinimumSize: 10,
				},
			},
		},
		Manifest:      s.minimalManifest(),
		ReferenceName: "foo",
		Source:        applicationcharm.CharmHubSource,
		Revision:      42,
		Architecture:  architecture.AMD64,
	}
	platform := deployment.Platform{
		Channel:      "24.04",
		OSType:       deployment.Ubuntu,
		Architecture: architecture.AMD64,
	}
	app := application.AddIAASApplicationArg{
		BaseAddApplicationArg: application.BaseAddApplicationArg{
			Charm: ch,
			CharmDownloadInfo: &applicationcharm.DownloadInfo{
				Provenance:         applicationcharm.ProvenanceDownload,
				CharmhubIdentifier: "foo",
				DownloadURL:        "https://example.com/foo",
				DownloadSize:       42,
			},
			Platform: platform,
			StorageDirectives: []application.CreateApplicationStorageDirectiveArg{
				{
					Count:        3,
					Name:         "data",
					Size:         10,
					ProviderType: ptr("fast"),
				},
			},
		},
	}

	s.provider.EXPECT().ConstraintsValidator(gomock.Any()).Return(nil, nil)
	s.provider.EXPECT().PrecheckInstance(gomock.Any(), environs.PrecheckInstanceParams{
		Base: corebase.Base{
			OS: "ubuntu",
			Channel: corebase.Channel{
				Track: "24.04",
			},
		},
	}).Return(nil)

	s.state.EXPECT().GetDefaultStorageProvisioners(gomock.Any()).Return(
		application.DefaultStorageProvisioners{
			BlockdeviceProviderType: ptr("fast"),
		}, nil,
	)
	s.state.EXPECT().CreateIAASApplication(gomock.Any(), "foo", gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, n string, a application.AddIAASApplicationArg, u []application.AddIAASUnitArg) (coreapplication.ID, []coremachine.Name, error) {
		c.Assert(a, tc.DeepEquals, app)
		mc := tc.NewMultiChecker()
		mc.AddExpr(`_[_].AddUnitArg.CreateUnitStorageArg`,
			s.createUnitStorageArgChecker(), tc.ExpectedValue)
		c.Assert(u, mc, us)
		return id, nil, nil
	})

	s.charm.EXPECT().Actions().Return(&charm.Actions{})
	s.charm.EXPECT().Config().Return(&charm.Config{})
	s.charm.EXPECT().Meta().Return(&charm.Meta{
		Name: "foo",
		Storage: map[string]charm.Storage{
			"data": {
				Name:        "data",
				Type:        charm.StorageBlock,
				Shared:      false,
				CountMin:    1,
				CountMax:    3,
				MinimumSize: 10,
			},
		},
	}).MinTimes(1)
	s.charm.EXPECT().Manifest().Return(&charm.Manifest{Bases: []charm.Base{{
		Name:          "ubuntu",
		Channel:       charm.Channel{Risk: charm.Stable},
		Architectures: []string{"amd64"},
	}}}).MinTimes(1)

	_, err := s.service.CreateIAASApplication(c.Context(), "foo", s.charm, corecharm.Origin{
		Source:   corecharm.CharmHub,
		Platform: corecharm.MustParsePlatform("amd64/ubuntu/24.04"),
		Revision: ptr(42),
	}, AddApplicationArgs{
		ReferenceName: "foo",
		DownloadInfo: &applicationcharm.DownloadInfo{
			Provenance:         applicationcharm.ProvenanceDownload,
			CharmhubIdentifier: "foo",
			DownloadURL:        "https://example.com/foo",
			DownloadSize:       42,
		},
		StorageDirectiveOverrides: map[string]ApplicationStorageDirectiveOverride{
			"data": {Count: ptr(uint32(3))},
		},
	}, AddIAASUnitArg{})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *providerServiceSuite) TestCreateIAASApplicationWithStorageFilesystem(c *tc.C) {
	defer s.setupMocks(c).Finish()

	id := coreapplication.GenID(c)

	now := ptr(s.clock.Now())
	us := []application.AddIAASUnitArg{{
		AddUnitArg: application.AddUnitArg{
			UnitStatusArg: application.UnitStatusArg{
				AgentStatus: &status.StatusInfo[status.UnitAgentStatusType]{
					Status: status.UnitAgentStatusAllocating,
					Since:  now,
				},
				WorkloadStatus: &status.StatusInfo[status.WorkloadStatusType]{
					Status:  status.WorkloadStatusWaiting,
					Message: "waiting for machine",
					Since:   now,
				},
			},
			CreateUnitStorageArg: application.CreateUnitStorageArg{
				StorageDirectives: []application.CreateUnitStorageDirectiveArg{
					{
						Name:         "data",
						Count:        1,
						Size:         10,
						ProviderType: ptr("rootfs"),
					},
				},
				StorageInstances: []application.CreateUnitStorageInstanceArg{
					{
						Name: "data",
					},
				},
				StorageToAttach: []storage.StorageInstanceUUID{""},
				StorageToOwn:    []storage.StorageInstanceUUID{""},
			},
		},
		Platform: deployment.Platform{
			OSType:  deployment.Ubuntu,
			Channel: "24.04",
		},
	}}
	ch := applicationcharm.Charm{
		Metadata: applicationcharm.Metadata{
			Name:  "foo",
			RunAs: "default",
			Storage: map[string]applicationcharm.Storage{
				"data": {
					Name:        "data",
					Type:        applicationcharm.StorageFilesystem,
					Shared:      false,
					CountMin:    1,
					CountMax:    2,
					MinimumSize: 10,
				},
			},
		},
		Manifest:      s.minimalManifest(),
		ReferenceName: "foo",
		Source:        applicationcharm.CharmHubSource,
		Revision:      42,
		Architecture:  architecture.AMD64,
	}
	platform := deployment.Platform{
		Channel:      "24.04",
		OSType:       deployment.Ubuntu,
		Architecture: architecture.AMD64,
	}
	app := application.AddIAASApplicationArg{
		BaseAddApplicationArg: application.BaseAddApplicationArg{
			Charm: ch,
			CharmDownloadInfo: &applicationcharm.DownloadInfo{
				Provenance:         applicationcharm.ProvenanceDownload,
				CharmhubIdentifier: "foo",
				DownloadURL:        "https://example.com/foo",
				DownloadSize:       42,
			},
			Platform: platform,
			StorageDirectives: []application.CreateApplicationStorageDirectiveArg{
				{
					Count:        1,
					Name:         "data",
					Size:         10,
					ProviderType: ptr("rootfs"),
				},
			},
		},
	}

	s.provider.EXPECT().ConstraintsValidator(gomock.Any()).Return(nil, nil)
	s.provider.EXPECT().PrecheckInstance(gomock.Any(), environs.PrecheckInstanceParams{
		Base: corebase.Base{
			OS: "ubuntu",
			Channel: corebase.Channel{
				Track: "24.04",
			},
		},
	}).Return(nil)

	s.state.EXPECT().GetDefaultStorageProvisioners(gomock.Any()).Return(
		application.DefaultStorageProvisioners{
			FilesystemProviderType: ptr("rootfs"),
		}, nil,
	)
	s.state.EXPECT().CreateIAASApplication(gomock.Any(), "foo", gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, n string, a application.AddIAASApplicationArg, u []application.AddIAASUnitArg) (coreapplication.ID, []coremachine.Name, error) {
		c.Assert(a, tc.DeepEquals, app)
		mc := tc.NewMultiChecker()
		mc.AddExpr(`_[_].AddUnitArg.CreateUnitStorageArg`,
			s.createUnitStorageArgChecker(), tc.ExpectedValue)
		c.Assert(u, mc, us)
		return id, nil, nil
	})

	s.charm.EXPECT().Actions().Return(&charm.Actions{})
	s.charm.EXPECT().Config().Return(&charm.Config{})
	s.charm.EXPECT().Meta().Return(&charm.Meta{
		Name: "foo",
		Storage: map[string]charm.Storage{
			"data": {
				Name:        "data",
				Type:        charm.StorageFilesystem,
				Shared:      false,
				CountMin:    1,
				CountMax:    2,
				MinimumSize: 10,
			},
		},
	}).MinTimes(1)
	s.charm.EXPECT().Manifest().Return(&charm.Manifest{Bases: []charm.Base{{
		Name:          "ubuntu",
		Channel:       charm.Channel{Risk: charm.Stable},
		Architectures: []string{"amd64"},
	}}}).MinTimes(1)

	_, err := s.service.CreateIAASApplication(c.Context(), "foo", s.charm, corecharm.Origin{
		Source:   corecharm.CharmHub,
		Platform: corecharm.MustParsePlatform("amd64/ubuntu/24.04"),
		Revision: ptr(42),
	}, AddApplicationArgs{
		ReferenceName: "foo",
		DownloadInfo: &applicationcharm.DownloadInfo{
			Provenance:         applicationcharm.ProvenanceDownload,
			CharmhubIdentifier: "foo",
			DownloadURL:        "https://example.com/foo",
			DownloadSize:       42,
		},
	}, AddIAASUnitArg{})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *providerServiceSuite) TestCreateIAASApplicationWithStorageFilesystemDefaultSource(c *tc.C) {
	defer s.setupMocks(c).Finish()

	id := coreapplication.GenID(c)

	now := ptr(s.clock.Now())
	us := []application.AddIAASUnitArg{{
		AddUnitArg: application.AddUnitArg{
			UnitStatusArg: application.UnitStatusArg{
				AgentStatus: &status.StatusInfo[status.UnitAgentStatusType]{
					Status: status.UnitAgentStatusAllocating,
					Since:  now,
				},
				WorkloadStatus: &status.StatusInfo[status.WorkloadStatusType]{
					Status:  status.WorkloadStatusWaiting,
					Message: "waiting for machine",
					Since:   now,
				},
			},
			CreateUnitStorageArg: application.CreateUnitStorageArg{
				StorageDirectives: []application.CreateUnitStorageDirectiveArg{
					{
						Name:         "data",
						Count:        2,
						Size:         10,
						ProviderType: ptr("fast"),
					},
				},
				StorageInstances: []application.CreateUnitStorageInstanceArg{
					{
						Name: "data",
					},
					{
						Name: "data",
					},
				},
				StorageToAttach: []storage.StorageInstanceUUID{"", ""},
				StorageToOwn:    []storage.StorageInstanceUUID{"", ""},
			},
		},
		Platform: deployment.Platform{
			OSType:  deployment.Ubuntu,
			Channel: "24.04",
		},
	}}
	ch := applicationcharm.Charm{
		Metadata: applicationcharm.Metadata{
			Name:  "foo",
			RunAs: "default",
			Storage: map[string]applicationcharm.Storage{
				"data": {
					Name:        "data",
					Type:        applicationcharm.StorageFilesystem,
					Shared:      false,
					CountMin:    1,
					CountMax:    3,
					MinimumSize: 10,
				},
			},
		},
		Manifest:      s.minimalManifest(),
		ReferenceName: "foo",
		Source:        applicationcharm.CharmHubSource,
		Revision:      42,
		Architecture:  architecture.AMD64,
	}
	platform := deployment.Platform{
		Channel:      "24.04",
		OSType:       deployment.Ubuntu,
		Architecture: architecture.AMD64,
	}
	app := application.AddIAASApplicationArg{
		BaseAddApplicationArg: application.BaseAddApplicationArg{
			Charm: ch,
			CharmDownloadInfo: &applicationcharm.DownloadInfo{
				Provenance:         applicationcharm.ProvenanceDownload,
				CharmhubIdentifier: "foo",
				DownloadURL:        "https://example.com/foo",
				DownloadSize:       42,
			},
			Platform: platform,
			StorageDirectives: []application.CreateApplicationStorageDirectiveArg{
				{
					Count:        2,
					Name:         "data",
					Size:         10,
					ProviderType: ptr("fast"),
				},
			},
		},
	}

	s.provider.EXPECT().ConstraintsValidator(gomock.Any()).Return(nil, nil)
	s.provider.EXPECT().PrecheckInstance(gomock.Any(), environs.PrecheckInstanceParams{
		Base: corebase.Base{
			OS: "ubuntu",
			Channel: corebase.Channel{
				Track: "24.04",
			},
		},
	}).Return(nil)

	s.state.EXPECT().GetDefaultStorageProvisioners(gomock.Any()).Return(
		application.DefaultStorageProvisioners{
			FilesystemProviderType: ptr("fast"),
		}, nil,
	)
	s.state.EXPECT().CreateIAASApplication(gomock.Any(), "foo", gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, n string, a application.AddIAASApplicationArg, u []application.AddIAASUnitArg) (coreapplication.ID, []coremachine.Name, error) {
		c.Assert(a, tc.DeepEquals, app)
		mc := tc.NewMultiChecker()
		mc.AddExpr(`_[_].AddUnitArg.CreateUnitStorageArg`,
			s.createUnitStorageArgChecker(), tc.ExpectedValue)
		c.Assert(u, mc, us)
		return id, nil, nil
	})

	s.charm.EXPECT().Actions().Return(&charm.Actions{})
	s.charm.EXPECT().Config().Return(&charm.Config{})
	s.charm.EXPECT().Meta().Return(&charm.Meta{
		Name: "foo",
		Storage: map[string]charm.Storage{
			"data": {
				Name:        "data",
				Type:        charm.StorageFilesystem,
				Shared:      false,
				CountMin:    1,
				CountMax:    3,
				MinimumSize: 10,
			},
		},
	}).MinTimes(1)
	s.charm.EXPECT().Manifest().Return(&charm.Manifest{Bases: []charm.Base{{
		Name:          "ubuntu",
		Channel:       charm.Channel{Risk: charm.Stable},
		Architectures: []string{"amd64"},
	}}}).MinTimes(1)

	_, err := s.service.CreateIAASApplication(c.Context(), "foo", s.charm, corecharm.Origin{
		Source:   corecharm.CharmHub,
		Platform: corecharm.MustParsePlatform("amd64/ubuntu/24.04"),
		Revision: ptr(42),
	}, AddApplicationArgs{
		ReferenceName: "foo",
		DownloadInfo: &applicationcharm.DownloadInfo{
			Provenance:         applicationcharm.ProvenanceDownload,
			CharmhubIdentifier: "foo",
			DownloadURL:        "https://example.com/foo",
			DownloadSize:       42,
		},
		StorageDirectiveOverrides: map[string]ApplicationStorageDirectiveOverride{
			"data": {Count: ptr(uint32(2))},
		},
	}, AddIAASUnitArg{})
	c.Assert(err, tc.ErrorIsNil)
}

// TestCreateIAASApplicationWithSharedStorage is testing that we can create an
// application for a charm that has shared storage. As long as the charm is ok
// with having no instances of the storage. We don't support shared storage but
// will continue to work if the charm can function without the storage. That is
// CountMin == 0.
func (s *providerServiceSuite) TestCreateIAASApplicationWithSharedStorage(c *tc.C) {
	defer s.setupMocks(c).Finish()

	id := coreapplication.GenID(c)

	platform := deployment.Platform{
		Channel:      "24.04",
		OSType:       deployment.Ubuntu,
		Architecture: architecture.AMD64,
	}

	now := ptr(s.clock.Now())
	us := []application.AddIAASUnitArg{{
		AddUnitArg: application.AddUnitArg{
			UnitStatusArg: application.UnitStatusArg{
				AgentStatus: &status.StatusInfo[status.UnitAgentStatusType]{
					Status: status.UnitAgentStatusAllocating,
					Since:  now,
				},
				WorkloadStatus: &status.StatusInfo[status.WorkloadStatusType]{
					Status:  status.WorkloadStatusWaiting,
					Message: "waiting for machine",
					Since:   now,
				},
			},
		},
		Platform: platform,
	}}
	ch := applicationcharm.Charm{
		Metadata: applicationcharm.Metadata{
			Name:  "foo",
			RunAs: "default",
			Storage: map[string]applicationcharm.Storage{
				"data": {
					Name:     "data",
					Type:     applicationcharm.StorageFilesystem,
					Shared:   true,
					CountMin: 0,
				},
			},
		},
		Manifest:      s.minimalManifest(),
		ReferenceName: "foo",
		Source:        applicationcharm.CharmHubSource,
		Revision:      42,
		Architecture:  architecture.AMD64,
	}
	app := application.AddIAASApplicationArg{
		BaseAddApplicationArg: application.BaseAddApplicationArg{
			Charm: ch,
			CharmDownloadInfo: &applicationcharm.DownloadInfo{
				Provenance:         applicationcharm.ProvenanceDownload,
				CharmhubIdentifier: "foo",
				DownloadURL:        "https://example.com/foo",
				DownloadSize:       42,
			},
			Platform:          platform,
			StorageDirectives: []application.CreateApplicationStorageDirectiveArg{},
		},
	}

	s.provider.EXPECT().PrecheckInstance(gomock.Any(), environs.PrecheckInstanceParams{
		Base: corebase.Base{
			OS: "ubuntu",
			Channel: corebase.Channel{
				Track: "24.04",
			},
		},
	}).Return(nil)

	s.provider.EXPECT().ConstraintsValidator(gomock.Any()).Return(nil, nil)
	s.state.EXPECT().GetDefaultStorageProvisioners(gomock.Any()).Return(
		application.DefaultStorageProvisioners{
			FilesystemProviderType: ptr("fast"),
		}, nil,
	)
	s.state.EXPECT().CreateIAASApplication(gomock.Any(), "foo", gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, n string, a application.AddIAASApplicationArg, u []application.AddIAASUnitArg) (coreapplication.ID, []coremachine.Name, error) {
		c.Assert(a, tc.DeepEquals, app)
		c.Assert(u, tc.DeepEquals, us)
		return id, nil, nil
	})

	s.charm.EXPECT().Actions().Return(&charm.Actions{})
	s.charm.EXPECT().Config().Return(&charm.Config{})
	s.charm.EXPECT().Meta().Return(&charm.Meta{
		Name: "foo",
		Storage: map[string]charm.Storage{
			"data": {
				Name:     "data",
				Type:     charm.StorageFilesystem,
				Shared:   true,
				CountMin: 0,
			},
		},
	}).MinTimes(1)
	s.charm.EXPECT().Manifest().Return(&charm.Manifest{Bases: []charm.Base{{
		Name:          "ubuntu",
		Channel:       charm.Channel{Risk: charm.Stable},
		Architectures: []string{"amd64"},
	}}}).MinTimes(1)

	_, err := s.service.CreateIAASApplication(c.Context(), "foo", s.charm, corecharm.Origin{
		Source:   corecharm.CharmHub,
		Platform: corecharm.MustParsePlatform("amd64/ubuntu/24.04"),
		Revision: ptr(42),
	}, AddApplicationArgs{
		ReferenceName: "foo",
		DownloadInfo: &applicationcharm.DownloadInfo{
			Provenance:         applicationcharm.ProvenanceDownload,
			CharmhubIdentifier: "foo",
			DownloadURL:        "https://example.com/foo",
			DownloadSize:       42,
		},
	}, AddIAASUnitArg{})
	c.Assert(err, tc.ErrorIsNil)
}

// TODO (tlm): Add this test back in.
//func (s *providerServiceSuite) TestCreateIAASApplicationWithStorageValidates(c *tc.C) {
//	defer s.setupMocks(c).Finish()
//
//	s.charm.EXPECT().Meta().Return(&charm.Meta{
//		Name: "mine",
//		Storage: map[string]charm.Storage{
//			"data": {
//				Name: "data",
//				Type: charm.StorageBlock,
//			},
//		},
//	}).MinTimes(1)
//	s.charm.EXPECT().Manifest().Return(&charm.Manifest{Bases: []charm.Base{{
//		Name:          "ubuntu",
//		Channel:       charm.Channel{Risk: charm.Beta},
//		Architectures: []string{"arm64"},
//	}}}).MinTimes(1)
//
//	_, err := s.service.CreateIAASApplication(c.Context(), "foo", s.charm, corecharm.Origin{
//		Platform: corecharm.MustParsePlatform("arm64/ubuntu/24.04"),
//	}, AddApplicationArgs{
//		ReferenceName: "foo",
//		DownloadInfo: &applicationcharm.DownloadInfo{
//			CharmhubIdentifier: "foo",
//			DownloadURL:        "https://example.com/foo",
//			DownloadSize:       42,
//		},
//		Storage: map[string]storage.Directive{
//			"logs": {Count: 2},
//		},
//	}, AddUnitArg{})
//	c.Assert(err, tc.ErrorMatches, `.*invalid storage directives: charm "mine" has no store called "logs"`)
//}

func (s *providerServiceSuite) TestDeviceConstraintsValidateNotInCharmMeta(c *tc.C) {
	deviceConstraints := map[string]devices.Constraints{
		"dev0": {
			Type:  "type0",
			Count: 42,
		},
	}
	charmMeta := &charm.Meta{
		Name: "foo",
		Devices: map[string]charm.Device{
			"dev1": {
				Description: "dev1 description",
				Type:        "type1",
				CountMin:    1,
			},
		},
	}

	err := validateDeviceConstraints(deviceConstraints, charmMeta)
	c.Assert(err, tc.ErrorMatches, "charm \"foo\" has no device called \"dev0\"")
}

func (s *providerServiceSuite) TestDeviceConstraintsValidateCount(c *tc.C) {
	deviceConstraints := map[string]devices.Constraints{
		"dev0": {
			Type:  "type0",
			Count: 42,
		},
	}
	charmMeta := &charm.Meta{
		Name: "foo",
		Devices: map[string]charm.Device{
			"dev0": {
				Description: "dev0 description",
				Type:        "type0",
				CountMin:    43,
			},
		},
	}

	err := validateDeviceConstraints(deviceConstraints, charmMeta)
	c.Assert(err, tc.ErrorMatches, "minimum device count is 43, 42 specified")
}

func (s *providerServiceSuite) TestDeviceConstraintsMissingFromMeta(c *tc.C) {
	deviceConstraints := map[string]devices.Constraints{
		"dev0": {
			Type:  "type0",
			Count: 43,
		},
	}
	charmMeta := &charm.Meta{
		Name: "foo",
		Devices: map[string]charm.Device{
			"dev0": {
				Description: "dev0 description",
				Type:        "type0",
				CountMin:    42,
			},
			"dev1": {
				Description: "dev1 description",
				Type:        "type1",
				CountMin:    1,
			},
		},
	}

	err := validateDeviceConstraints(deviceConstraints, charmMeta)
	c.Assert(err, tc.ErrorMatches, "no constraints specified for device \"dev1\"")
}

func (s *providerServiceSuite) TestDeviceConstraintsValid(c *tc.C) {
	deviceConstraints := map[string]devices.Constraints{
		"dev0": {
			Type:  "type0",
			Count: 43,
		},
		"dev1": {
			Type:  "type1",
			Count: 2,
		},
	}
	charmMeta := &charm.Meta{
		Name: "foo",
		Devices: map[string]charm.Device{
			"dev0": {
				Description: "dev0 description",
				Type:        "type0",
				CountMin:    42,
			},
			"dev1": {
				Description: "dev1 description",
				Type:        "type1",
				CountMin:    1,
			},
		},
	}

	err := validateDeviceConstraints(deviceConstraints, charmMeta)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *providerServiceSuite) TestGetSupportedFeatures(c *tc.C) {
	defer s.setupMocks(c).Finish()

	agentVersion := semversion.MustParse("4.0.0")
	s.agentVersionGetter.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(agentVersion, nil)

	s.caasProvider.EXPECT().SupportedFeatures().Return(assumes.FeatureSet{}, nil)

	features, err := s.service.GetSupportedFeatures(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	var fs assumes.FeatureSet
	fs.Add(assumes.Feature{
		Name:        "juju",
		Description: assumes.UserFriendlyFeatureDescriptions["juju"],
		Version:     &agentVersion,
	})
	c.Check(features, tc.DeepEquals, fs)
}

func (s *providerServiceSuite) TestGetSupportedFeaturesNotSupported(c *tc.C) {
	ctrl := s.setupMocksWithProvider(c, providerNotSupported, providerNotSupported)
	defer ctrl.Finish()

	agentVersion := semversion.MustParse("4.0.0")
	s.agentVersionGetter.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(agentVersion, nil)

	features, err := s.service.GetSupportedFeatures(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	var fs assumes.FeatureSet
	fs.Add(assumes.Feature{
		Name:        "juju",
		Description: assumes.UserFriendlyFeatureDescriptions["juju"],
		Version:     &agentVersion,
	})
	c.Check(features, tc.DeepEquals, fs)
}

func (s *providerServiceSuite) TestGetApplicationConstraintsInvalidAppID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service.GetApplicationConstraints(c.Context(), "bad-app-id")
	c.Assert(err, tc.ErrorMatches, "application ID: id \"bad-app-id\" not valid")
}

func (s *providerServiceSuite) TestSetApplicationConstraintsInvalidAppID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service.SetApplicationConstraints(c.Context(), "bad-app-id", coreconstraints.Value{})
	c.Assert(err, tc.ErrorMatches, "application ID: id \"bad-app-id\" not valid")
}

func (s *providerServiceSuite) TestSetConstraintsProviderNotSupported(c *tc.C) {
	ctrl := s.setupMocksWithProvider(c, providerNotSupported, providerNotSupported)
	defer ctrl.Finish()

	id := coreapplication.GenID(c)

	s.state.EXPECT().SetApplicationConstraints(gomock.Any(), id, constraints.Constraints{}).Return(nil)

	err := s.service.SetApplicationConstraints(c.Context(), id, coreconstraints.Value{})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *providerServiceSuite) TestSetConstraintsValidatorError(c *tc.C) {
	ctrl := s.setupMocksWithProvider(c, noProviderError, noProviderError)
	defer ctrl.Finish()

	id := coreapplication.GenID(c)

	s.provider.EXPECT().ConstraintsValidator(gomock.Any()).Return(nil, errors.New("boom"))

	err := s.service.SetApplicationConstraints(c.Context(), id, coreconstraints.Value{})
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *providerServiceSuite) TestSetConstraintsValidateError(c *tc.C) {
	ctrl := s.setupMocksWithProvider(c, noProviderError, noProviderError)
	defer ctrl.Finish()

	id := coreapplication.GenID(c)

	validator := NewMockValidator(ctrl)
	s.provider.EXPECT().ConstraintsValidator(gomock.Any()).Return(validator, nil)
	validator.EXPECT().Validate(gomock.Any()).Return(nil, errors.New("boom"))

	err := s.service.SetApplicationConstraints(c.Context(), id, coreconstraints.Value{})
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *providerServiceSuite) TestSetConstraintsUnsupportedValues(c *tc.C) {
	ctrl := s.setupMocksWithProvider(c, noProviderError, noProviderError)
	defer ctrl.Finish()

	id := coreapplication.GenID(c)

	validator := NewMockValidator(ctrl)
	s.provider.EXPECT().ConstraintsValidator(gomock.Any()).Return(validator, nil)
	validator.EXPECT().Validate(gomock.Any()).Return([]string{"arch", "mem"}, nil)
	s.state.EXPECT().SetApplicationConstraints(gomock.Any(), id, constraints.Constraints{Arch: ptr("amd64"), Mem: ptr(uint64(8))}).Return(nil)

	err := s.service.SetApplicationConstraints(c.Context(), id, coreconstraints.Value{Arch: ptr("amd64"), Mem: ptr(uint64(8))})
	c.Assert(err, tc.ErrorIsNil)
	//c.Check(c.GetTestLog(), tc.Contains, "unsupported constraints: arch,mem")
}

func (s *providerServiceSuite) TestSetConstraints(c *tc.C) {
	ctrl := s.setupMocksWithProvider(c, noProviderError, noProviderError)
	defer ctrl.Finish()

	defer s.setupMocks(c).Finish()

	id := coreapplication.GenID(c)

	validator := NewMockValidator(ctrl)
	s.provider.EXPECT().ConstraintsValidator(gomock.Any()).Return(validator, nil)
	validator.EXPECT().Validate(gomock.Any()).Return(nil, nil)
	s.state.EXPECT().SetApplicationConstraints(gomock.Any(), id, constraints.Constraints{Arch: ptr("amd64"), Mem: ptr(uint64(8))}).Return(nil)

	err := s.service.SetApplicationConstraints(c.Context(), id, coreconstraints.Value{Arch: ptr("amd64"), Mem: ptr(uint64(8))})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *providerServiceSuite) TestAddCAASUnitsEmptyConstraints(c *tc.C) {
	ctrl := s.setupMocksWithProvider(c, noProviderError, noProviderError)
	defer ctrl.Finish()

	appUUID := coreapplication.GenID(c)

	now := ptr(s.clock.Now())
	u := []application.AddCAASUnitArg{{
		AddUnitArg: application.AddUnitArg{
			UnitStatusArg: application.UnitStatusArg{
				AgentStatus: &status.StatusInfo[status.UnitAgentStatusType]{
					Status: status.UnitAgentStatusAllocating,
					Since:  now,
				},
				WorkloadStatus: &status.StatusInfo[status.WorkloadStatusType]{
					Status:  status.WorkloadStatusWaiting,
					Message: corestatus.MessageInstallingAgent,
					Since:   now,
				},
			},
			Constraints: constraints.Constraints{
				Arch: ptr(arch.AMD64),
			},
		},
	}}
	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), "ubuntu").Return(appUUID, nil)
	returnedCharm := applicationcharm.Charm{
		Metadata: applicationcharm.Metadata{
			Subordinate: false,
		},
	}
	s.state.EXPECT().GetCharmByApplicationID(gomock.Any(), appUUID).Return(returnedCharm, nil)
	s.state.EXPECT().GetApplicationStorageDirectives(gomock.Any(), appUUID).Return(nil, nil)
	s.state.EXPECT().GetApplicationCharmOrigin(gomock.Any(), appUUID).Return(application.CharmOrigin{}, nil)
	s.provider.EXPECT().PrecheckInstance(gomock.Any(), environs.PrecheckInstanceParams{
		Base: corebase.Base{
			OS: "ubuntu",
		},
		Constraints: coreconstraints.MustParse("arch=amd64"),
	}).Return(nil)
	s.expectEmptyUnitConstraints(c, appUUID)

	var received []application.AddCAASUnitArg
	s.state.EXPECT().AddCAASUnits(gomock.Any(), appUUID, gomock.Any()).DoAndReturn(func(_ context.Context, _ coreapplication.ID, args ...application.AddCAASUnitArg) ([]coreunit.Name, error) {
		received = args
		return []coreunit.Name{"foo/0"}, nil
	})

	unitNames, err := s.service.AddCAASUnits(c.Context(), "ubuntu", AddUnitArg{})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(received, tc.DeepEquals, u)
	c.Check(unitNames, tc.HasLen, 1)
	c.Check(unitNames[0], tc.Equals, coreunit.Name("foo/0"))
}

func (s *providerServiceSuite) TestAddCAASUnitsAppConstraints(c *tc.C) {
	ctrl := s.setupMocksWithProvider(c, noProviderError, noProviderError)
	defer ctrl.Finish()

	appUUID := coreapplication.GenID(c)
	unitUUID := coreunit.GenUUID(c)

	now := ptr(s.clock.Now())
	u := []application.AddCAASUnitArg{{
		AddUnitArg: application.AddUnitArg{
			Constraints: constraints.Constraints{
				Arch:           ptr("amd64"),
				Container:      ptr(instance.LXD),
				CpuCores:       ptr(uint64(4)),
				Mem:            ptr(uint64(1024)),
				RootDisk:       ptr(uint64(1024)),
				RootDiskSource: ptr("root-disk-source"),
				Tags:           ptr([]string{"tag1", "tag2"}),
				InstanceRole:   ptr("instance-role"),
				InstanceType:   ptr("instance-type"),
				Spaces: ptr([]constraints.SpaceConstraint{
					{SpaceName: "space1", Exclude: false},
				}),
				VirtType:         ptr("virt-type"),
				Zones:            ptr([]string{"zone1", "zone2"}),
				AllocatePublicIP: ptr(true),
			},
			UnitStatusArg: application.UnitStatusArg{
				AgentStatus: &status.StatusInfo[status.UnitAgentStatusType]{
					Status: status.UnitAgentStatusAllocating,
					Since:  now,
				},
				WorkloadStatus: &status.StatusInfo[status.WorkloadStatusType]{
					Status:  status.WorkloadStatusWaiting,
					Message: corestatus.MessageInstallingAgent,
					Since:   now,
				},
			},
			Placement: deployment.Placement{
				Type:      deployment.PlacementTypeMachine,
				Directive: "0/lxd/0",
			},
		},
	}}
	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), "ubuntu").Return(appUUID, nil)
	returnedCharm := applicationcharm.Charm{
		Metadata: applicationcharm.Metadata{
			Subordinate: false,
		},
	}
	s.state.EXPECT().GetCharmByApplicationID(gomock.Any(), appUUID).Return(returnedCharm, nil)
	s.state.EXPECT().GetApplicationStorageDirectives(gomock.Any(), appUUID).Return(nil, nil)
	s.state.EXPECT().GetApplicationCharmOrigin(gomock.Any(), appUUID).Return(application.CharmOrigin{}, nil)
	s.provider.EXPECT().PrecheckInstance(gomock.Any(), environs.PrecheckInstanceParams{
		Base: corebase.Base{
			OS: "ubuntu",
		},
		Constraints: coreconstraints.MustParse("arch=amd64 container=lxd cores=4 instance-role=instance-role instance-type=instance-type mem=1024M root-disk=1024M root-disk-source=root-disk-source tags=tag1,tag2 spaces=space1 virt-type=virt-type zones=zone1,zone2 allocate-public-ip=true"),
	}).Return(nil)
	s.expectAppConstraints(c, unitUUID, appUUID)

	var received []application.AddCAASUnitArg
	s.state.EXPECT().AddCAASUnits(gomock.Any(), appUUID, gomock.Any()).DoAndReturn(func(_ context.Context, _ coreapplication.ID, args ...application.AddCAASUnitArg) ([]coreunit.Name, error) {
		received = args
		return []coreunit.Name{"foo/0"}, nil
	})

	a := AddUnitArg{
		Placement: instance.MustParsePlacement("0/lxd/0"),
	}
	unitNames, err := s.service.AddCAASUnits(c.Context(), "ubuntu", a)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(received, tc.DeepEquals, u)
	c.Check(unitNames, tc.HasLen, 1)
	c.Check(unitNames[0], tc.Equals, coreunit.Name("foo/0"))
}

func (s *providerServiceSuite) TestAddCAASUnitsModelConstraints(c *tc.C) {
	ctrl := s.setupMocksWithProvider(c, noProviderError, noProviderError)
	defer ctrl.Finish()

	appUUID := coreapplication.GenID(c)
	unitUUID := coreunit.GenUUID(c)

	now := ptr(s.clock.Now())
	u := []application.AddCAASUnitArg{{
		AddUnitArg: application.AddUnitArg{
			Constraints: constraints.Constraints{
				Arch:           ptr("amd64"),
				Container:      ptr(instance.LXD),
				CpuCores:       ptr(uint64(4)),
				Mem:            ptr(uint64(1024)),
				RootDisk:       ptr(uint64(1024)),
				RootDiskSource: ptr("root-disk-source"),
				Tags:           ptr([]string{"tag1", "tag2"}),
				InstanceRole:   ptr("instance-role"),
				InstanceType:   ptr("instance-type"),
				Spaces: ptr([]constraints.SpaceConstraint{
					{SpaceName: "space1", Exclude: false},
				}),
				VirtType:         ptr("virt-type"),
				Zones:            ptr([]string{"zone1", "zone2"}),
				AllocatePublicIP: ptr(true),
			},
			UnitStatusArg: application.UnitStatusArg{
				AgentStatus: &status.StatusInfo[status.UnitAgentStatusType]{
					Status: status.UnitAgentStatusAllocating,
					Since:  now,
				},
				WorkloadStatus: &status.StatusInfo[status.WorkloadStatusType]{
					Status:  status.WorkloadStatusWaiting,
					Message: corestatus.MessageInstallingAgent,
					Since:   now,
				},
			},
		},
	}}
	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), "ubuntu").Return(appUUID, nil)
	returnedCharm := applicationcharm.Charm{
		Metadata: applicationcharm.Metadata{
			Subordinate: false,
		},
	}
	s.state.EXPECT().GetCharmByApplicationID(gomock.Any(), appUUID).Return(returnedCharm, nil)
	s.state.EXPECT().GetApplicationStorageDirectives(gomock.Any(), appUUID).Return(nil, nil)
	s.state.EXPECT().GetApplicationCharmOrigin(gomock.Any(), appUUID).Return(application.CharmOrigin{}, nil)
	s.provider.EXPECT().PrecheckInstance(gomock.Any(), environs.PrecheckInstanceParams{
		Base: corebase.Base{
			OS: "ubuntu",
		},
		Constraints: coreconstraints.MustParse("arch=amd64 container=lxd cores=4 instance-role=instance-role instance-type=instance-type mem=1024M root-disk=1024M root-disk-source=root-disk-source tags=tag1,tag2 spaces=space1 virt-type=virt-type zones=zone1,zone2 allocate-public-ip=true"),
	}).Return(nil)
	s.expectModelConstraints(c, unitUUID, appUUID)

	var received []application.AddCAASUnitArg
	s.state.EXPECT().AddCAASUnits(gomock.Any(), appUUID, gomock.Any()).DoAndReturn(func(_ context.Context, _ coreapplication.ID, args ...application.AddCAASUnitArg) ([]coreunit.Name, error) {
		received = args
		return []coreunit.Name{"foo/0"}, nil
	})

	unitNames, err := s.service.AddCAASUnits(c.Context(), "ubuntu", AddUnitArg{})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(received, tc.DeepEquals, u)
	c.Check(unitNames, tc.HasLen, 1)
	c.Check(unitNames[0], tc.Equals, coreunit.Name("foo/0"))
}

func (s *providerServiceSuite) TestAddCAASUnitsFullConstraints(c *tc.C) {
	ctrl := s.setupMocksWithProvider(c, noProviderError, noProviderError)
	defer ctrl.Finish()

	appUUID := coreapplication.GenID(c)
	unitUUID := coreunit.GenUUID(c)

	now := ptr(s.clock.Now())
	u := []application.AddCAASUnitArg{{
		AddUnitArg: application.AddUnitArg{
			Constraints: constraints.Constraints{
				Arch:     ptr(arch.AMD64),
				CpuCores: ptr(uint64(4)),
				CpuPower: ptr(uint64(75)),
			},
			UnitStatusArg: application.UnitStatusArg{
				AgentStatus: &status.StatusInfo[status.UnitAgentStatusType]{
					Status: status.UnitAgentStatusAllocating,
					Since:  now,
				},
				WorkloadStatus: &status.StatusInfo[status.WorkloadStatusType]{
					Status:  status.WorkloadStatusWaiting,
					Message: corestatus.MessageInstallingAgent,
					Since:   now,
				},
			},
		},
	}}
	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), "ubuntu").Return(appUUID, nil)
	returnedCharm := applicationcharm.Charm{
		Metadata: applicationcharm.Metadata{
			Subordinate: false,
		},
	}
	s.state.EXPECT().GetCharmByApplicationID(gomock.Any(), appUUID).Return(returnedCharm, nil)
	s.state.EXPECT().GetApplicationStorageDirectives(gomock.Any(), appUUID).Return(nil, nil)
	s.state.EXPECT().GetApplicationCharmOrigin(gomock.Any(), appUUID).Return(application.CharmOrigin{}, nil)
	s.provider.EXPECT().PrecheckInstance(gomock.Any(), environs.PrecheckInstanceParams{
		Base: corebase.Base{
			OS: "ubuntu",
		},
		Constraints: coreconstraints.MustParse("arch=amd64 cores=4 cpu-power=75"),
	}).Return(nil)
	s.expectFullConstraints(c, unitUUID, appUUID)

	var received []application.AddCAASUnitArg
	s.state.EXPECT().AddCAASUnits(gomock.Any(), appUUID, gomock.Any()).DoAndReturn(func(_ context.Context, _ coreapplication.ID, args ...application.AddCAASUnitArg) ([]coreunit.Name, error) {
		received = args
		return []coreunit.Name{"foo/0"}, nil
	})

	unitNames, err := s.service.AddCAASUnits(c.Context(), "ubuntu", AddUnitArg{})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(received, tc.DeepEquals, u)
	c.Check(unitNames, tc.HasLen, 1)
	c.Check(unitNames[0], tc.Equals, coreunit.Name("foo/0"))
}

func (s *providerServiceSuite) TestAddIAASUnitsInvalidName(c *tc.C) {
	ctrl := s.setupMocksWithProvider(c, noProviderError, noProviderError)
	defer ctrl.Finish()

	_, _, err := s.service.AddIAASUnits(c.Context(), "!!!", AddIAASUnitArg{})
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNameNotValid)
}

func (s *providerServiceSuite) TestAddIAASUnitsNoUnits(c *tc.C) {
	ctrl := s.setupMocksWithProvider(c, noProviderError, noProviderError)
	defer ctrl.Finish()

	units, _, err := s.service.AddIAASUnits(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(units, tc.HasLen, 0)
}

func (s *providerServiceSuite) TestAddIAASUnitsApplicationNotFound(c *tc.C) {
	ctrl := s.setupMocksWithProvider(c, noProviderError, noProviderError)
	defer ctrl.Finish()

	appUUID := coreapplication.GenID(c)

	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), "ubuntu").Return(appUUID, applicationerrors.ApplicationNotFound)

	_, _, err := s.service.AddIAASUnits(c.Context(), "ubuntu", AddIAASUnitArg{})
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *providerServiceSuite) TestAddIAASUnitsInvalidPlacement(c *tc.C) {
	ctrl := s.setupMocksWithProvider(c, noProviderError, noProviderError)
	defer ctrl.Finish()

	appUUID := coreapplication.GenID(c)
	unitUUID := coreunit.GenUUID(c)

	s.state.EXPECT().GetApplicationStorageDirectives(gomock.Any(), appUUID).Return(nil, nil)
	s.state.EXPECT().GetApplicationCharmOrigin(gomock.Any(), appUUID).Return(application.CharmOrigin{}, nil)
	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), "ubuntu").Return(appUUID, nil)
	returnedCharm := applicationcharm.Charm{
		Metadata: applicationcharm.Metadata{
			Subordinate: false,
		},
	}
	s.state.EXPECT().GetCharmByApplicationID(gomock.Any(), appUUID).Return(returnedCharm, nil)
	s.expectFullConstraints(c, unitUUID, appUUID)

	placement := &instance.Placement{
		Scope:     instance.MachineScope,
		Directive: "0/kvm/0",
	}

	a := AddIAASUnitArg{
		AddUnitArg: AddUnitArg{
			Placement: placement,
		},
	}
	_, _, err := s.service.AddIAASUnits(c.Context(), "ubuntu", a)
	c.Assert(err, tc.ErrorMatches, ".*invalid placement.*")
}

func (s *providerServiceSuite) TestAddIAASUnitsMachinePlacement(c *tc.C) {
	ctrl := s.setupMocksWithProvider(c, noProviderError, noProviderError)
	defer ctrl.Finish()

	appUUID := coreapplication.GenID(c)
	unitUUID := coreunit.GenUUID(c)

	s.state.EXPECT().GetApplicationStorageDirectives(gomock.Any(), appUUID).Return(nil, nil)
	s.state.EXPECT().GetApplicationCharmOrigin(gomock.Any(), appUUID).Return(application.CharmOrigin{
		Platform: deployment.Platform{
			OSType:  deployment.Ubuntu,
			Channel: "24.04",
		},
	}, nil)
	s.provider.EXPECT().PrecheckInstance(gomock.Any(), environs.PrecheckInstanceParams{
		Constraints: coreconstraints.MustParse("cores=4 cpu-power=75 arch=amd64"),
		Base: corebase.Base{
			OS: "ubuntu",
			Channel: corebase.Channel{
				Track: "24.04",
			},
		},
	}).Return(nil)

	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), "ubuntu").Return(appUUID, nil)
	returnedCharm := applicationcharm.Charm{
		Metadata: applicationcharm.Metadata{
			Subordinate: false,
		},
	}
	s.state.EXPECT().GetCharmByApplicationID(gomock.Any(), appUUID).Return(returnedCharm, nil)
	s.expectFullConstraints(c, unitUUID, appUUID)

	s.state.EXPECT().AddIAASUnits(gomock.Any(), appUUID, gomock.Any()).Return([]coreunit.Name{"foo/0"}, nil, nil)

	placement := &instance.Placement{
		Scope:     instance.MachineScope,
		Directive: "0",
	}

	a := AddIAASUnitArg{
		AddUnitArg: AddUnitArg{
			Placement: placement,
		},
	}
	_, _, err := s.service.AddIAASUnits(c.Context(), "ubuntu", a)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *providerServiceSuite) TestMergeApplicationAndModelConstraintsNotSupported(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.provider.EXPECT().ConstraintsValidator(gomock.Any()).Return(s.validator, errors.Errorf("not supported %w", coreerrors.NotSupported))

	_, err := s.service.mergeApplicationAndModelConstraints(c.Context(), constraints.Constraints{}, false)
	c.Assert(err, tc.ErrorIs, coreerrors.NotSupported)
}

func (s *providerServiceSuite) TestMergeApplicationAndModelConstraintsNilValidator(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.provider.EXPECT().ConstraintsValidator(gomock.Any()).Return(nil, nil)

	cons, err := s.service.mergeApplicationAndModelConstraints(c.Context(), constraints.Constraints{}, false)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cons, tc.DeepEquals, coreconstraints.Value{})
}

func (s *providerServiceSuite) TestMergeApplicationAndModelConstraintsConstraintsNotFound(c *tc.C) {
	ctrl := s.setupMocksWithProvider(c, noProviderError, noProviderError)
	defer ctrl.Finish()

	s.provider.EXPECT().ConstraintsValidator(gomock.Any()).Return(s.validator, nil)

	s.state.EXPECT().GetModelConstraints(gomock.Any()).Return(constraints.Constraints{}, modelerrors.ConstraintsNotFound)

	s.validator.EXPECT().Merge(
		constraints.EncodeConstraints(constraints.Constraints{}),
		constraints.EncodeConstraints(constraints.Constraints{})).
		Return(coreconstraints.Value{}, nil)

	_, err := s.service.mergeApplicationAndModelConstraints(c.Context(), constraints.Constraints{}, false)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *providerServiceSuite) TestMergeApplicationAndModelConstraintsNotSubordinateWithArch(c *tc.C) {
	ctrl := s.setupMocksWithProvider(c, noProviderError, noProviderError)
	defer ctrl.Finish()

	s.provider.EXPECT().ConstraintsValidator(gomock.Any()).Return(s.validator, nil)

	s.state.EXPECT().GetModelConstraints(gomock.Any()).Return(constraints.Constraints{}, modelerrors.ConstraintsNotFound)

	s.validator.EXPECT().Merge(
		constraints.EncodeConstraints(constraints.Constraints{}),
		constraints.EncodeConstraints(constraints.Constraints{
			Arch: ptr(arch.AMD64),
		})).
		Return(coreconstraints.Value{
			Arch: ptr(arch.AMD64),
		}, nil)

	merged, err := s.service.mergeApplicationAndModelConstraints(c.Context(), constraints.Constraints{
		Arch: ptr(arch.AMD64),
	}, false)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(*merged.Arch, tc.Equals, arch.AMD64)
}

func (s *providerServiceSuite) TestMergeApplicationAndModelConstraintsSubordinateWithArch(c *tc.C) {
	ctrl := s.setupMocksWithProvider(c, noProviderError, noProviderError)
	defer ctrl.Finish()

	s.provider.EXPECT().ConstraintsValidator(gomock.Any()).Return(s.validator, nil)

	s.state.EXPECT().GetModelConstraints(gomock.Any()).Return(constraints.Constraints{
		RootDiskSource: ptr("source-disk"),
		Mem:            ptr(uint64(42)),
	}, modelerrors.ConstraintsNotFound)

	s.validator.EXPECT().Merge(
		constraints.EncodeConstraints(constraints.Constraints{
			RootDiskSource: ptr("source-disk"),
			Mem:            ptr(uint64(42)),
		}),
		constraints.EncodeConstraints(constraints.Constraints{
			Arch: ptr(arch.AMD64),
		})).
		Return(coreconstraints.Value{
			Arch:           ptr(arch.AMD64),
			RootDiskSource: ptr("source-disk"),
			Mem:            ptr(uint64(42)),
		}, nil)

	merged, err := s.service.mergeApplicationAndModelConstraints(c.Context(), constraints.Constraints{
		Arch: ptr(arch.AMD64),
	}, true)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(*merged.Arch, tc.Equals, arch.AMD64)
	c.Check(*merged.RootDiskSource, tc.Equals, "source-disk")
	c.Check(*merged.Mem, tc.Equals, uint64(42))
}

func (s *providerServiceSuite) TestMergeApplicationAndModelConstraintsNotSubordinateWithoutArch(c *tc.C) {
	ctrl := s.setupMocksWithProvider(c, noProviderError, noProviderError)
	defer ctrl.Finish()

	s.provider.EXPECT().ConstraintsValidator(gomock.Any()).Return(s.validator, nil)

	s.state.EXPECT().GetModelConstraints(gomock.Any()).Return(constraints.Constraints{
		Mem: ptr(uint64(42)),
	}, modelerrors.ConstraintsNotFound)

	s.validator.EXPECT().Merge(
		constraints.EncodeConstraints(constraints.Constraints{
			Mem: ptr(uint64(42)),
		}),
		constraints.EncodeConstraints(constraints.Constraints{
			RootDiskSource: ptr("source-disk"),
		})).
		Return(coreconstraints.Value{
			RootDiskSource: ptr("source-disk"),
			Mem:            ptr(uint64(42)),
		}, nil)

	merged, err := s.service.mergeApplicationAndModelConstraints(c.Context(), constraints.Constraints{
		RootDiskSource: ptr("source-disk"),
	}, false)
	c.Assert(err, tc.ErrorIsNil)
	// Default arch should be added in this case.
	c.Check(*merged.Arch, tc.Equals, arch.AMD64)
	c.Check(*merged.RootDiskSource, tc.Equals, "source-disk")
	c.Check(*merged.Mem, tc.Equals, uint64(42))
}

func (s *providerServiceSuite) expectEmptyUnitConstraints(c *tc.C, appUUID coreapplication.ID) {
	appConstraints := constraints.Constraints{}
	modelConstraints := constraints.Constraints{}

	s.provider.EXPECT().ConstraintsValidator(gomock.Any()).Return(s.validator, nil)

	s.state.EXPECT().GetApplicationConstraints(gomock.Any(), appUUID).Return(appConstraints, nil)
	s.state.EXPECT().GetModelConstraints(gomock.Any()).Return(modelConstraints, nil)

	s.validator.EXPECT().Merge(constraints.EncodeConstraints(appConstraints), constraints.EncodeConstraints(modelConstraints)).Return(coreconstraints.Value{}, nil)
}

func (s *providerServiceSuite) expectAppConstraints(c *tc.C, unitUUID coreunit.UUID, appUUID coreapplication.ID) {
	appConstraints := constraints.Constraints{
		Arch:           ptr("amd64"),
		Container:      ptr(instance.LXD),
		CpuCores:       ptr(uint64(4)),
		Mem:            ptr(uint64(1024)),
		RootDisk:       ptr(uint64(1024)),
		RootDiskSource: ptr("root-disk-source"),
		Tags:           ptr([]string{"tag1", "tag2"}),
		InstanceRole:   ptr("instance-role"),
		InstanceType:   ptr("instance-type"),
		Spaces: ptr([]constraints.SpaceConstraint{
			{SpaceName: "space1", Exclude: false},
		}),
		VirtType:         ptr("virt-type"),
		Zones:            ptr([]string{"zone1", "zone2"}),
		AllocatePublicIP: ptr(true),
	}
	modelConstraints := constraints.Constraints{}
	unitConstraints := appConstraints

	s.validator.EXPECT().Merge(constraints.EncodeConstraints(modelConstraints), constraints.EncodeConstraints(appConstraints)).
		Return(constraints.EncodeConstraints(unitConstraints), nil)

	s.provider.EXPECT().ConstraintsValidator(gomock.Any()).Return(s.validator, nil)

	s.state.EXPECT().GetApplicationConstraints(gomock.Any(), appUUID).Return(appConstraints, nil)
	s.state.EXPECT().GetModelConstraints(gomock.Any()).Return(modelConstraints, nil)
}

func (s *providerServiceSuite) expectModelConstraints(c *tc.C, unitUUID coreunit.UUID, appUUID coreapplication.ID) {
	modelConstraints := constraints.Constraints{
		Arch:           ptr("amd64"),
		Container:      ptr(instance.LXD),
		CpuCores:       ptr(uint64(4)),
		Mem:            ptr(uint64(1024)),
		RootDisk:       ptr(uint64(1024)),
		RootDiskSource: ptr("root-disk-source"),
		Tags:           ptr([]string{"tag1", "tag2"}),
		InstanceRole:   ptr("instance-role"),
		InstanceType:   ptr("instance-type"),
		Spaces: ptr([]constraints.SpaceConstraint{
			{SpaceName: "space1", Exclude: false},
		}),
		VirtType:         ptr("virt-type"),
		Zones:            ptr([]string{"zone1", "zone2"}),
		AllocatePublicIP: ptr(true),
	}
	appConstraints := constraints.Constraints{}
	unitConstraints := modelConstraints

	s.provider.EXPECT().ConstraintsValidator(gomock.Any()).Return(s.validator, nil)

	s.state.EXPECT().GetApplicationConstraints(gomock.Any(), appUUID).Return(appConstraints, nil)
	s.state.EXPECT().GetModelConstraints(gomock.Any()).Return(modelConstraints, nil)

	s.validator.EXPECT().Merge(constraints.EncodeConstraints(modelConstraints), constraints.EncodeConstraints(appConstraints)).Return(constraints.EncodeConstraints(unitConstraints), nil)
}

func (s *providerServiceSuite) expectFullConstraints(c *tc.C, unitUUID coreunit.UUID, appUUID coreapplication.ID) {
	modelConstraints := constraints.Constraints{
		CpuCores: ptr(uint64(4)),
	}
	appConstraints := constraints.Constraints{
		CpuPower: ptr(uint64(75)),
	}
	unitConstraints := constraints.Constraints{
		CpuCores: ptr(uint64(4)),
		CpuPower: ptr(uint64(75)),
	}

	s.provider.EXPECT().ConstraintsValidator(gomock.Any()).Return(s.validator, nil)

	s.validator.EXPECT().Merge(constraints.EncodeConstraints(modelConstraints), constraints.EncodeConstraints(appConstraints)).Return(constraints.EncodeConstraints(unitConstraints), nil)

	s.state.EXPECT().GetApplicationConstraints(gomock.Any(), appUUID).Return(appConstraints, nil)
	s.state.EXPECT().GetModelConstraints(gomock.Any()).Return(modelConstraints, nil)
}

func (s *providerServiceSuite) createUnitStorageArgChecker() tc.Checker {
	mc := tc.NewMultiChecker()
	mc.AddExpr(`_.StorageInstances[_].UUID`, tc.IsNonZeroUUID)
	mc.AddExpr(`_.StorageInstances[_].FilesystemUUID`, tc.Deref(tc.IsNonZeroUUID))
	mc.AddExpr(`_.StorageToAttach[_]`, tc.IsNonZeroUUID)
	mc.AddExpr(`_.StorageToOwn[_]`, tc.IsNonZeroUUID)
	return mc
}

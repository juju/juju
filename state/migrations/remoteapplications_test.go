// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrations

import (
	"github.com/juju/description/v9"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
)

type RemoteApplicationsExportSuite struct{}

var _ = gc.Suite(&RemoteApplicationsExportSuite{})

func (s *RemoteApplicationsExportSuite) TestExportRemoteApplication(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	entities := []MigrationRemoteApplication{
		s.migrationRemoteApplication(ctrl, func(expect *MockMigrationRemoteApplicationMockRecorder) {
			expect.Tag().Return(names.NewApplicationTag("uuid-1"))
			expect.OfferUUID().Return("offer-uuid")
			expect.URL().Return("me/model.foo", true)
			expect.SourceModel().Return(names.NewModelTag("uuid-2"))
			expect.IsConsumerProxy().Return(false)
			expect.Macaroon().Return("mac")
			expect.ConsumeVersion().Return(1)
			expect.Bindings().Return(map[string]string{
				"binding-key": "binding-value",
			})
			// Return the endpoint mocks
			expect.Endpoints().Return([]MigrationRemoteEndpoint{
				{
					Name:      "app-uuid-1-endpoint-1",
					Role:      "role",
					Interface: "db",
				},
			}, nil)
			// Return the spaces mocks
			expect.Spaces().Return([]MigrationRemoteSpace{
				{
					Name:       "app-uuid-1-spaces-1",
					CloudType:  "aws",
					ProviderId: "provider-id-1",
					ProviderAttributes: map[string]interface{}{
						"attr-1": "value-1",
					},
					Subnets: []MigrationRemoteSubnet{
						{
							CIDR:              "10.0.0.1/24",
							ProviderId:        "provider-id-2",
							VLANTag:           1,
							AvailabilityZones: []string{"eu-west-1"},
							ProviderSpaceId:   "provider-space-id",
							ProviderNetworkId: "provider-network-id",
						},
					},
				},
			})

			expect.GlobalKey().Return("c#app-uuid-1")
		}),
	}

	remoteSpace := NewMockRemoteSpace(ctrl)
	remoteSpace.EXPECT().AddSubnet(description.SubnetArgs{
		CIDR:              "10.0.0.1/24",
		ProviderId:        "provider-id-2",
		VLANTag:           1,
		AvailabilityZones: []string{"eu-west-1"},
		ProviderSpaceId:   "provider-space-id",
		ProviderNetworkId: "provider-network-id",
	})

	remoteApplication := NewMockRemoteApplication(ctrl)
	remoteApplication.EXPECT().SetStatus(description.StatusArgs{
		Value: "status-value",
	})
	remoteApplication.EXPECT().AddEndpoint(description.RemoteEndpointArgs{
		Name:      "app-uuid-1-endpoint-1",
		Role:      "role",
		Interface: "db",
	})
	remoteApplication.EXPECT().AddSpace(description.RemoteSpaceArgs{
		Name:       "app-uuid-1-spaces-1",
		CloudType:  "aws",
		ProviderId: "provider-id-1",
		ProviderAttributes: map[string]interface{}{
			"attr-1": "value-1",
		},
	}).Return(remoteSpace)

	source := NewMockRemoteApplicationSource(ctrl)
	source.EXPECT().AllRemoteApplications().Return(entities, nil)
	source.EXPECT().StatusArgs("c#app-uuid-1").Return(description.StatusArgs{
		Value: "status-value",
	}, nil)

	model := NewMockRemoteApplicationModel(ctrl)
	model.EXPECT().AddRemoteApplication(description.RemoteApplicationArgs{
		Tag:             names.NewApplicationTag("uuid-1"),
		OfferUUID:       "offer-uuid",
		URL:             "me/model.foo",
		SourceModel:     names.NewModelTag("uuid-2"),
		IsConsumerProxy: false,
		Macaroon:        "mac",
		ConsumeVersion:  1,
		Bindings: map[string]string{
			"binding-key": "binding-value",
		},
	}).Return(remoteApplication)

	migration := ExportRemoteApplications{}
	err := migration.Execute(source, model)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *RemoteApplicationsExportSuite) TestExportRemoteApplicationWithSourceFailure(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	source := NewMockRemoteApplicationSource(ctrl)
	source.EXPECT().AllRemoteApplications().Return(nil, errors.New("fail"))

	model := NewMockRemoteApplicationModel(ctrl)

	migration := ExportRemoteApplications{}
	err := migration.Execute(source, model)
	c.Assert(err, gc.ErrorMatches, "fail")
}

func (s *RemoteApplicationsExportSuite) TestExportRemoteApplicationWithEndpointsFailure(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	entities := []MigrationRemoteApplication{
		s.migrationRemoteApplication(ctrl, func(expect *MockMigrationRemoteApplicationMockRecorder) {
			expect.Tag().Return(names.NewApplicationTag("uuid-1"))
			expect.OfferUUID().Return("offer-uuid")
			expect.URL().Return("me/model.foo", true)
			expect.SourceModel().Return(names.NewModelTag("uuid-2"))
			expect.IsConsumerProxy().Return(false)
			expect.Macaroon().Return("mac")
			expect.ConsumeVersion().Return(1)
			expect.Bindings().Return(map[string]string{
				"binding-key": "binding-value",
			})
			// Return the endpoint mocks
			expect.Endpoints().Return(nil, errors.New("fail"))
			expect.GlobalKey().Return("c#app-uuid-1")
		}),
	}

	remoteApplication := NewMockRemoteApplication(ctrl)
	remoteApplication.EXPECT().SetStatus(description.StatusArgs{
		Value: "status-value",
	})

	source := NewMockRemoteApplicationSource(ctrl)
	source.EXPECT().AllRemoteApplications().Return(entities, nil)
	source.EXPECT().StatusArgs("c#app-uuid-1").Return(description.StatusArgs{
		Value: "status-value",
	}, nil)

	model := NewMockRemoteApplicationModel(ctrl)
	model.EXPECT().AddRemoteApplication(description.RemoteApplicationArgs{
		Tag:             names.NewApplicationTag("uuid-1"),
		OfferUUID:       "offer-uuid",
		URL:             "me/model.foo",
		SourceModel:     names.NewModelTag("uuid-2"),
		IsConsumerProxy: false,
		Macaroon:        "mac",
		ConsumeVersion:  1,
		Bindings: map[string]string{
			"binding-key": "binding-value",
		},
	}).Return(remoteApplication)

	migration := ExportRemoteApplications{}
	err := migration.Execute(source, model)
	c.Assert(err, gc.ErrorMatches, "fail")
}

func (s *RemoteApplicationsExportSuite) TestExportRemoteApplicationWithStatusArgsFailure(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	entities := []MigrationRemoteApplication{
		s.migrationRemoteApplication(ctrl, func(expect *MockMigrationRemoteApplicationMockRecorder) {
			expect.Tag().Return(names.NewApplicationTag("uuid-1"))
			expect.OfferUUID().Return("offer-uuid")
			expect.URL().Return("me/model.foo", true)
			expect.SourceModel().Return(names.NewModelTag("uuid-2"))
			expect.IsConsumerProxy().Return(false)
			expect.Macaroon().Return("mac")
			expect.ConsumeVersion().Return(1)
			expect.Bindings().Return(map[string]string{
				"binding-key": "binding-value",
			})

			expect.GlobalKey().Return("c#app-uuid-1")
		}),
	}

	remoteApplication := NewMockRemoteApplication(ctrl)

	source := NewMockRemoteApplicationSource(ctrl)
	source.EXPECT().AllRemoteApplications().Return(entities, nil)
	source.EXPECT().StatusArgs("c#app-uuid-1").Return(description.StatusArgs{
		Value: "status-value",
	}, errors.New("fail"))

	model := NewMockRemoteApplicationModel(ctrl)
	model.EXPECT().AddRemoteApplication(description.RemoteApplicationArgs{
		Tag:             names.NewApplicationTag("uuid-1"),
		OfferUUID:       "offer-uuid",
		URL:             "me/model.foo",
		SourceModel:     names.NewModelTag("uuid-2"),
		IsConsumerProxy: false,
		Macaroon:        "mac",
		ConsumeVersion:  1,
		Bindings: map[string]string{
			"binding-key": "binding-value",
		},
	}).Return(remoteApplication)

	migration := ExportRemoteApplications{}
	err := migration.Execute(source, model)
	c.Assert(err, gc.ErrorMatches, "fail")
}

func (s *RemoteApplicationsExportSuite) migrationRemoteApplication(
	ctrl *gomock.Controller, fn func(expect *MockMigrationRemoteApplicationMockRecorder),
) *MockMigrationRemoteApplication {
	entity := NewMockMigrationRemoteApplication(ctrl)
	fn(entity.EXPECT())
	return entity
}

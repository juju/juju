// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrations

import (
	"github.com/juju/description/v6"
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
			// Return the endpoint mocks
			expect.Endpoints().Return([]MigrationRemoteEndpoint{
				{
					Name:      "app-uuid-1-endpoint-1",
					Role:      "role",
					Interface: "db",
				},
			}, nil)

			expect.GlobalKey().Return("c#app-uuid-1")
		}),
	}

	remoteApplication := NewMockRemoteApplication(ctrl)
	remoteApplication.EXPECT().SetStatus(description.StatusArgs{
		Value: "status-value",
	})
	remoteApplication.EXPECT().AddEndpoint(description.RemoteEndpointArgs{
		Name:      "app-uuid-1-endpoint-1",
		Role:      "role",
		Interface: "db",
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

// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/description/v8"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/logger/testing"
)

type importSuite struct {
	importService *MockImportService
}

var _ = gc.Suite(&importSuite{})

func (s *importSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.importService = NewMockImportService(ctrl)
	return ctrl
}

func (i *importSuite) TestApplicationSave(c *gc.C) {
	model := description.NewModel(description.ModelArgs{})

	appArgs := description.ApplicationArgs{
		Tag:      names.NewApplicationTag("prometheus"),
		CharmURL: "ch:prometheus-1",
	}
	app := model.AddApplication(appArgs)
	app.AddUnit(description.UnitArgs{
		Tag:          names.NewUnitTag("prometheus/0"),
		PasswordHash: "passwordhash",
		CloudContainer: &description.CloudContainerArgs{
			ProviderId: "provider-id",
			Address: description.AddressArgs{
				Value:   "10.6.6.6",
				Type:    "ipv4",
				Scope:   "local-machine",
				Origin:  "provider",
				SpaceID: "666",
			},
			Ports: []string{"6666"},
		},
	})
	app.SetCharmOrigin(description.CharmOriginArgs{
		Source:   "charm-hub",
		ID:       "1234",
		Hash:     "deadbeef",
		Revision: 1,
		Channel:  "666/stable",
		Platform: "arm64/ubuntu/24.04",
	})

	defer i.setupMocks(c).Finish()

	rev := 1
	i.importService.EXPECT().CreateApplication(
		gomock.Any(),
		"prometheus",
		&stubCharm{
			name:     "prometheus",
			revision: 1,
		},
		corecharm.Origin{
			Source:   "charm-hub",
			Type:     "charm",
			ID:       "1234",
			Hash:     "deadbeef",
			Revision: &rev,
			Channel: &charm.Channel{
				Track: "666",
				Risk:  "stable",
			},
			Platform: corecharm.Platform{
				Architecture: "arm64",
				OS:           "ubuntu",
				Channel:      "24.04",
			},
		},
		service.AddApplicationArgs{},
		[]service.AddUnitArg{{
			UnitName:     ptr("prometheus/0"),
			PasswordHash: ptr("passwordhash"),
			CloudContainer: ptr(service.CloudContainerParams{
				ProviderId: ptr("provider-id"),
				Address: ptr(network.SpaceAddress{
					MachineAddress: network.MachineAddress{
						Value: "10.6.6.6",
						Type:  "ipv4",
						Scope: "local-machine",
					},
					SpaceID: "666",
				}),
				AddressOrigin: ptr(network.OriginProvider),
				Ports:         ptr([]string{"6666"}),
			}),
		}},
	).Return("", nil)

	importOp := importOperation{
		service:      i.importService,
		logger:       testing.WrapCheckLog(c),
		charmOrigins: make(map[string]*corecharm.Origin),
	}

	err := importOp.Execute(context.Background(), model)
	c.Assert(err, jc.ErrorIsNil)
}

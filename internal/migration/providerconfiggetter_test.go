// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"testing"

	"github.com/juju/description/v11"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/cloud"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/providertracker"
	"github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/uuid"
)

type ephemeralProviderConfigSuite struct {
	testhelpers.IsolationSuite
	cloudService          *MockCloudService
	providerGetter        *MockProviderConfigServicesGetter
	providerConfigService *MockProviderConfigServices
}

func TestGetEphemeralProviderConfigSuite(t *testing.T) {
	tc.Run(t, &ephemeralProviderConfigSuite{})
}

func (s *ephemeralProviderConfigSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.cloudService = NewMockCloudService(ctrl)
	s.providerGetter = NewMockProviderConfigServicesGetter(ctrl)
	s.providerConfigService = NewMockProviderConfigServices(ctrl)
	s.providerGetter.EXPECT().ServicesForModel(gomock.Any(), gomock.Any()).Return(s.providerConfigService, nil)
	s.providerConfigService.EXPECT().Cloud().Return(s.cloudService)

	c.Cleanup(func() {
		s.cloudService = nil
		s.providerGetter = nil
		s.providerConfigService = nil
	})

	return ctrl
}

func (s *ephemeralProviderConfigSuite) TestGetEphemeralProviderConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange controller cloud domain data
	cloudName := "test-cloud"
	cloudRegion := "test-region"
	cloudResult := &cloud.Cloud{
		Name: cloudName,
		Type: "kubernetes",
		Regions: []cloud.Region{
			{
				Name: cloudRegion,
			},
		},
	}
	s.cloudService.EXPECT().Cloud(gomock.Any(), cloudName).Return(cloudResult, nil)

	// Arrange model description
	modelUUID := tc.Must(c, uuid.NewUUID).String()
	model := description.NewModel(description.ModelArgs{
		Type: "caas",
		Config: map[string]any{
			"name":       "test-model",
			"type":       "caas",
			"uuid":       modelUUID,
			"apt-mirror": "http://mirror",
		},
		Cloud:       cloudName,
		CloudRegion: cloudRegion,
	})
	modelConfig, err := config.New(config.NoDefaults,
		map[string]any{
			"name":       "test-model",
			"type":       "caas",
			"uuid":       modelUUID,
			"apt-mirror": "http://mirror",
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	model.SetCloudCredential(description.CloudCredentialArgs{
		Owner:    "owner",
		Cloud:    cloudName,
		Name:     "cred-name",
		AuthType: "access-key",
		Attributes: map[string]string{
			"attr1": "val1",
		},
	})

	// Arrange: struct under test
	controllerUUID := tc.Must(c, uuid.NewUUID)
	epcp, err := newEphemeralProviderConfigGetter(controllerUUID.String(), model, s.providerGetter)
	c.Assert(err, tc.ErrorIsNil)

	// Act
	cfg, err := epcp.GetEphemeralProviderConfig(c.Context())

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	cred := cloud.NewCredential("access-key", map[string]string{"attr1": "val1"})
	cred.Label = "cred-name"
	c.Check(cfg, tc.DeepEquals, providertracker.EphemeralProviderConfig{
		ModelType:   coremodel.CAAS,
		ModelConfig: modelConfig,
		CloudSpec: cloudspec.CloudSpec{
			Type:       "kubernetes",
			Name:       cloudName,
			Region:     cloudRegion,
			Credential: &cred,
		},
		ControllerUUID: controllerUUID,
	})
}

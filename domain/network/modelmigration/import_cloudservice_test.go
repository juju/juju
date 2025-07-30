// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"errors"
	"maps"
	"slices"
	"testing"

	"github.com/juju/collections/transform"
	"github.com/juju/description/v10"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/domain/network/internal"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type importCloudServiceSuite struct {
	importService    *MockImportService
	migrationService *MockMigrationService
}

func TestImportCloudServiceSuite(t *testing.T) {
	tc.Run(t, &importCloudServiceSuite{})
}

func (s *importCloudServiceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.importService = NewMockImportService(ctrl)
	s.migrationService = NewMockMigrationService(ctrl)

	c.Cleanup(func() {
		s.importService = nil
		s.migrationService = nil
	})

	return ctrl
}

func (s *importCloudServiceSuite) newImportOperation(c *tc.C) *importCloudServiceOperation {
	return &importCloudServiceOperation{
		migrationService: s.migrationService,
		logger:           loggertesting.WrapCheckLog(c),
	}
}

func (s *importCloudServiceSuite) TestImportCloudService(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	model := description.NewModel(description.ModelArgs{
		Type: description.CAAS,
	})

	app1 := model.AddApplication(description.ApplicationArgs{
		Name: "app-1",
	})
	app2 := model.AddApplication(description.ApplicationArgs{
		Name: "app-2",
	})
	app1.SetCloudService(description.CloudServiceArgs{
		ProviderId: "app-1-service",
		Addresses: []description.AddressArgs{
			{
				Value:   "192.0.2.1",
				Type:    "ipv4",
				Scope:   "public",
				Origin:  "provider",
				SpaceID: "space-1",
			},
			{
				Value:   "2001:db8::1",
				Type:    "ipv6",
				Scope:   "public",
				Origin:  "provider",
				SpaceID: "space-1",
			},
		},
	})
	app2.SetCloudService(description.CloudServiceArgs{
		ProviderId: "app-2-service",
		Addresses: []description.AddressArgs{
			{
				Value:   "192.0.2.2",
				Type:    "ipv4",
				Scope:   "public",
				Origin:  "provider",
				SpaceID: "space-2",
			},
		},
	})
	args := []internal.ImportCloudService{
		{
			ApplicationName: "app-1",
			ProviderID:      "app-1-service",
			Addresses: []internal.ImportCloudServiceAddress{
				{
					Value:   "192.0.2.1",
					Type:    "ipv4",
					Scope:   "public",
					Origin:  "provider",
					SpaceID: "space-1",
				},
				{
					Value:   "2001:db8::1",
					Type:    "ipv6",
					Scope:   "public",
					Origin:  "provider",
					SpaceID: "space-1",
				},
			},
		},
		{
			ApplicationName: "app-2",
			ProviderID:      "app-2-service",
			Addresses: []internal.ImportCloudServiceAddress{{
				Value:   "192.0.2.2",
				Type:    "ipv4",
				Scope:   "public",
				Origin:  "provider",
				SpaceID: "space-2",
			}},
		},
	}
	s.migrationService.EXPECT().ImportCloudServices(gomock.Any(), cloudServiceMatcher{
		c:        c,
		expected: args,
	}).Return(nil)

	// Act
	err := s.newImportOperation(c).Execute(c.Context(), model)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importCloudServiceSuite) TestImportCloudServiceIaaS(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	model := description.NewModel(description.ModelArgs{
		Type: description.IAAS,
	})

	// No expectations on ImportCloudServices since it shouldn't be called for IaaS

	// Act
	err := s.newImportOperation(c).Execute(c.Context(), model)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importCloudServiceSuite) TestImportCloudServiceError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	model := description.NewModel(description.ModelArgs{
		Type: description.CAAS,
	})

	expectedError := errors.New("import cloud services failed")
	s.migrationService.EXPECT().ImportCloudServices(gomock.Any(), gomock.Any()).Return(expectedError)

	// Act
	err := s.newImportOperation(c).Execute(c.Context(), model)

	// Assert
	c.Assert(err, tc.ErrorMatches, "importing cloud services: import cloud services failed")
}

type cloudServiceMatcher struct {
	c        *tc.C
	expected []internal.ImportCloudService
}

func (m cloudServiceMatcher) Matches(x interface{}) bool {
	input, ok := x.([]internal.ImportCloudService)
	if !ok {
		return false
	}
	noAddresses := func(in internal.ImportCloudService) internal.ImportCloudService {
		in.Addresses = nil
		return in
	}
	mapAddresses := func(in internal.ImportCloudService) (string, []internal.ImportCloudServiceAddress) {
		return in.ProviderID, in.Addresses
	}
	inputAddresses := transform.SliceToMap(input, mapAddresses)
	expectedAddresses := transform.SliceToMap(m.expected, mapAddresses)

	m.c.Check(slices.Collect(maps.Keys(inputAddresses)), tc.SameContents, slices.Collect(maps.Keys(expectedAddresses)))
	for k, in := range inputAddresses {
		// UUIDs are assigned in the code under test. Ensure they exist, then
		// remove it to enable SameContents checks over the other fields.
		in = transform.Slice(in, func(in internal.ImportCloudServiceAddress) internal.ImportCloudServiceAddress {
			m.c.Check(in.UUID, tc.Not(tc.Equals), "")
			in.UUID = ""
			return in
		})
		m.c.Check(in, tc.SameContents, expectedAddresses[k], tc.Commentf("for %+v", k))
	}

	// UUIDs are assigned in the code under test. Ensure they exist, then
	// remove it to enable SameContents checks over the other fields.
	input = transform.Slice(input, func(in internal.ImportCloudService) internal.ImportCloudService {
		m.c.Check(in.UUID, tc.Not(tc.Equals), "")
		m.c.Check(in.DeviceUUID, tc.Not(tc.Equals), "")
		m.c.Check(in.NetNodeUUID, tc.Not(tc.Equals), "")
		in.UUID, in.DeviceUUID, in.NetNodeUUID = "", "", ""
		return in
	})

	return m.c.Check(
		transform.Slice(input, noAddresses),
		tc.SameContents,
		transform.Slice(m.expected, noAddresses))
}

func (cloudServiceMatcher) String() string {
	return "matches args for ImportCloudService"
}

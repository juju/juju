// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"errors"
	"maps"
	"slices"
	"testing"

	"github.com/juju/collections/transform"
	"github.com/juju/description/v12"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/domain/network/internal"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type importK8sServiceSuite struct {
	migrationService *MockK8sServiceMigrationService
}

func TestImportK8sServiceSuite(t *testing.T) {
	tc.Run(t, &importK8sServiceSuite{})
}

func (s *importK8sServiceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.migrationService = NewMockK8sServiceMigrationService(ctrl)

	c.Cleanup(func() {
		s.migrationService = nil
	})

	return ctrl
}

func (s *importK8sServiceSuite) newImportOperation(c *tc.C) *importK8sServiceOperation {
	return &importK8sServiceOperation{
		migrationService: s.migrationService,
		logger:           loggertesting.WrapCheckLog(c),
	}
}

func (s *importK8sServiceSuite) TestImportK8sService(c *tc.C) {
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
	app3 := model.AddApplication(description.ApplicationArgs{
		Name: "app-3",
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
			}, {
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
	app3.SetCloudService(description.CloudServiceArgs{
		ProviderId: "app-3-service",
	})
	args := []internal.ImportK8sService{
		{
			ApplicationName: "app-1",
			ProviderID:      "app-1-service",
			Addresses: []internal.ImportK8sServiceAddress{
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
		}, {
			ApplicationName: "app-2",
			ProviderID:      "app-2-service",
			Addresses: []internal.ImportK8sServiceAddress{{
				Value:   "192.0.2.2",
				Type:    "ipv4",
				Scope:   "public",
				Origin:  "provider",
				SpaceID: "space-2",
			}},
		}, {
			ApplicationName: "app-3",
			ProviderID:      "app-3-service",
		},
	}
	s.migrationService.EXPECT().ImportK8sServices(gomock.Any(), k8sServiceMatcher{
		c:        c,
		expected: args,
	}).Return(nil)

	// Act
	err := s.newImportOperation(c).Execute(c.Context(), model)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importK8sServiceSuite) TestImportK8sServiceIaaS(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	model := description.NewModel(description.ModelArgs{
		Type: description.IAAS,
	})

	// No expectations on ImportK8sServices since it shouldn't be called for IaaS

	// Act
	err := s.newImportOperation(c).Execute(c.Context(), model)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importK8sServiceSuite) TestImportK8sServiceError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	model := description.NewModel(description.ModelArgs{
		Type: description.CAAS,
	})

	expectedError := errors.New("import cloud services failed")
	s.migrationService.EXPECT().ImportK8sServices(gomock.Any(), gomock.Any()).Return(expectedError)

	// Act
	err := s.newImportOperation(c).Execute(c.Context(), model)

	// Assert
	c.Assert(err, tc.ErrorMatches, "importing cloud services: import cloud services failed")
}

type k8sServiceMatcher struct {
	c        *tc.C
	expected []internal.ImportK8sService
}

func (m k8sServiceMatcher) Matches(x any) bool {
	input, ok := x.([]internal.ImportK8sService)
	if !ok {
		return false
	}
	noAddresses := func(in internal.ImportK8sService) internal.ImportK8sService {
		in.Addresses = nil
		return in
	}
	mapAddresses := func(in internal.ImportK8sService) (string, []internal.ImportK8sServiceAddress) {
		return in.ProviderID, in.Addresses
	}
	inputAddresses := transform.SliceToMap(input, mapAddresses)
	expectedAddresses := transform.SliceToMap(m.expected, mapAddresses)

	m.c.Check(slices.Collect(maps.Keys(inputAddresses)), tc.SameContents, slices.Collect(maps.Keys(expectedAddresses)))
	for k, in := range inputAddresses {
		// UUIDs are assigned in the code under test. Ensure they exist, then
		// remove it to enable SameContents checks over the other fields.
		in = transform.Slice(in, func(in internal.ImportK8sServiceAddress) internal.ImportK8sServiceAddress {
			m.c.Check(in.UUID, tc.Not(tc.Equals), "")
			in.UUID = ""
			return in
		})
		m.c.Check(in, tc.SameContents, expectedAddresses[k], tc.Commentf("for %+v", k))
	}

	// UUIDs are assigned in the code under test. Ensure they exist, then
	// remove it to enable SameContents checks over the other fields.
	input = transform.Slice(input, func(in internal.ImportK8sService) internal.ImportK8sService {
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

func (k8sServiceMatcher) String() string {
	return "matches args for ImportK8sService"
}

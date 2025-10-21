// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"testing"

	"github.com/juju/clock"
	"github.com/juju/tc"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/application/internal"
	"github.com/juju/juju/domain/deployment"
	schematesting "github.com/juju/juju/domain/schema/testing"
	domainstorage "github.com/juju/juju/domain/storage"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

// applicationStorageSuite tests storage directives associated with an
// application.
type applicationStorageSuite struct {
	schematesting.ModelSuite
	storageHelper
}

// TestApplicationStorageSuite runs all of the tests located within the
// [applicationStorageSuite].
func TestApplicationStorageSuite(t *testing.T) {
	suite := &applicationStorageSuite{}
	suite.storageHelper.dbGetter = &suite.ModelSuite
	tc.Run(t, suite)
}

// createApplicationWithStorageDirectives creates an application with the
// supplied storage directive information.
func (s *applicationStorageSuite) createApplicationWithStorageDirectives(
	c *tc.C,
	charmName string,
	charmStorage map[string]charm.Storage,
	directives []internal.CreateApplicationStorageDirectiveArg,
) coreapplication.UUID {
	state := NewState(
		s.ModelSuite.TxnRunnerFactory(),
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)
	platform := deployment.Platform{
		Channel:      "22.04/stable",
		OSType:       deployment.Ubuntu,
		Architecture: architecture.ARM64,
	}
	channel := &deployment.Channel{
		Track:  "track",
		Risk:   "stable",
		Branch: "branch",
	}
	ctx := c.Context()

	appID, _, err := state.CreateIAASApplication(
		ctx, "test-app", application.AddIAASApplicationArg{
			BaseAddApplicationArg: application.BaseAddApplicationArg{
				Platform: platform,
				Channel:  channel,
				Charm: charm.Charm{
					Metadata: charm.Metadata{
						Name: charmName,
						Provides: map[string]charm.Relation{
							"endpoint": {
								Name:  "endpoint",
								Role:  charm.RoleProvider,
								Scope: charm.ScopeGlobal,
							},
							"misc": {
								Name:  "misc",
								Role:  charm.RoleProvider,
								Scope: charm.ScopeGlobal,
							},
						},
						ExtraBindings: map[string]charm.ExtraBinding{
							"extra": {
								Name: "extra",
							},
						},
						Storage: charmStorage,
					},
					Manifest: charm.Manifest{
						Bases: []charm.Base{
							{
								Name: "ubuntu",
								Channel: charm.Channel{
									Risk: charm.RiskStable,
								},
								Architectures: []string{"amd64"},
							},
						},
					},
					ReferenceName: charmName,
					Source:        charm.CharmHubSource,
					Revision:      42,
					Hash:          "hash",
				},
				CharmDownloadInfo: &charm.DownloadInfo{
					Provenance:         charm.ProvenanceDownload,
					CharmhubIdentifier: "ident",
					DownloadURL:        "https://example.com",
					DownloadSize:       42,
				},
				StorageDirectives: directives,
			},
		}, nil)
	c.Assert(err, tc.ErrorIsNil)

	return appID
}

func (s *applicationStorageSuite) TestGetApplicationStorageDirectives(c *tc.C) {
	poolUUID := s.storageHelper.newStoragePool(c, "test", "testprovider")
	appUUID := s.createApplicationWithStorageDirectives(
		c,
		"testcharm",
		map[string]charm.Storage{
			"str1": {
				CountMax: 10,
				CountMin: 2,
				Name:     "str1",
				Type:     charm.StorageFilesystem,
			},
			"str2": {
				CountMax: 1,
				CountMin: 0,
				Name:     "str2",
				Type:     charm.StorageFilesystem,
			},
		},
		[]internal.CreateApplicationStorageDirectiveArg{
			{
				Count:    2,
				Name:     domainstorage.Name("str1"),
				PoolUUID: poolUUID,
				Size:     1024,
			},
			{
				Count:    0,
				Name:     domainstorage.Name("str2"),
				PoolUUID: poolUUID,
				Size:     1024,
			},
		},
	)

	st := NewState(
		s.ModelSuite.TxnRunnerFactory(),
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)

	expected := []application.StorageDirective{
		{
			CharmMetadataName: "testcharm",
			CharmStorageType:  charm.StorageFilesystem,
			Count:             2,
			MaxCount:          10,
			Name:              domainstorage.Name("str1"),
			PoolUUID:          poolUUID,
			Size:              1024,
		},
		{
			CharmMetadataName: "testcharm",
			CharmStorageType:  charm.StorageFilesystem,
			Count:             0,
			MaxCount:          1,
			Name:              domainstorage.Name("str2"),
			PoolUUID:          poolUUID,
			Size:              1024,
		},
	}

	directives, err := st.GetApplicationStorageDirectives(c.Context(), appUUID)
	c.Check(err, tc.ErrorIsNil)
	c.Check(directives, tc.DeepEquals, expected)
}

// TestGetApplicationStorageDirectivesEmpty tests that when an application has
// no storage directives an empty result is returned.
func (s *applicationStorageSuite) TestGetApplicationStorageDirectivesEmpty(c *tc.C) {
	appUUID := s.createApplicationWithStorageDirectives(
		c,
		"testcharm",
		map[string]charm.Storage{},
		[]internal.CreateApplicationStorageDirectiveArg{},
	)

	st := NewState(
		s.ModelSuite.TxnRunnerFactory(),
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)

	directives, err := st.GetApplicationStorageDirectives(c.Context(), appUUID)
	c.Check(err, tc.ErrorIsNil)
	c.Check(directives, tc.HasLen, 0)
}

// TestGetApplicationStorageDirectivesNotFound tests that when an application
// is not found the appropriate error is returned.
func (s *applicationStorageSuite) TestGetApplicationStorageDirectivesNotFound(c *tc.C) {
	appUUID := tc.Must(c, coreapplication.NewUUID)
	st := NewState(
		s.ModelSuite.TxnRunnerFactory(),
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)

	_, err := st.GetApplicationStorageDirectives(c.Context(), appUUID)
	c.Check(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

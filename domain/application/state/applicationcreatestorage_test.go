// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"testing"

	"github.com/juju/clock"
	"github.com/juju/tc"

	coremachine "github.com/juju/juju/core/machine"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/deployment"
	domainlife "github.com/juju/juju/domain/life"
	domainnetwork "github.com/juju/juju/domain/network"
	domainstorage "github.com/juju/juju/domain/storage"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

// applicationCreateStorageSuite is a test suite for specifically testing
// storage when creating new applications.
type applicationCreateStorageSuite struct {
	baseSuite
	storageHelper

	state *State
}

// TestApplicationCreateStorageSuite runs all of the tests contained within
// [applicationCreateStorageSuite].
func TestApplicationCreateStorageSuite(t *testing.T) {
	suite := &applicationCreateStorageSuite{}
	suite.storageHelper.dbGetter = &suite.ModelSuite
	tc.Run(t, suite)
}

func (s *applicationCreateStorageSuite) SetUpTest(c *tc.C) {
	s.baseSuite.SetUpTest(c)

	s.state = NewState(
		s.TxnRunnerFactory(),
		s.modelUUID,
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)
}

func (s *applicationCreateStorageSuite) TearDownTest(c *tc.C) {
	s.state = nil
	s.baseSuite.TearDownTest(c)
}

// TestCreateIAASApplicationStorageInstanceNotFound verifies that
// CreateIAASApplication fails when an expected storage instance UUID does not
// exist.
func (s *applicationCreateStorageSuite) TestCreateIAASApplicationStorageInstanceNotFound(c *tc.C) {
	platform := deployment.Platform{
		Channel:      "666",
		OSType:       deployment.Ubuntu,
		Architecture: architecture.ARM64,
	}
	channel := &deployment.Channel{
		Track:  "track",
		Risk:   "risk",
		Branch: "branch",
	}

	unitUUID := tc.Must(c, coreunit.NewUUID)
	netNodeUUID := tc.Must(c, domainnetwork.NewNetNodeUUID)
	machineUUID := tc.Must(c, coremachine.NewUUID)
	existingSIUUID := s.newStorageInstanceWithName(c, "st1")
	notFoundSIUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)

	units := []application.AddIAASUnitArg{
		{
			AddUnitArg: application.AddUnitArg{
				CreateUnitStorageArg: domainstorage.CreateUnitStorageArg{
					ExistingStorageInstanceUUIDsToCheck: []domainstorage.StorageInstanceUUID{
						existingSIUUID,
						notFoundSIUUID,
					},
				},
				NetNodeUUID: netNodeUUID,
				UnitUUID:    unitUUID,
			},
			Platform:           platform,
			MachineNetNodeUUID: netNodeUUID,
			MachineUUID:        machineUUID,
		},
	}

	_, _, err := s.state.CreateIAASApplication(
		c.Context(),
		"app",
		application.AddIAASApplicationArg{
			BaseAddApplicationArg: application.BaseAddApplicationArg{
				Platform: platform,
				Charm: charm.Charm{
					Metadata:      s.minimalMetadata(c, "app"),
					Manifest:      s.minimalManifest(c),
					Source:        charm.CharmHubSource,
					ReferenceName: "app",
					Revision:      42,
					Architecture:  architecture.ARM64,
				},
				CharmDownloadInfo: &charm.DownloadInfo{
					Provenance:         charm.ProvenanceDownload,
					CharmhubIdentifier: "ident-1",
					DownloadURL:        "http://example.com/charm",
					DownloadSize:       666,
				},
				Channel: channel,
			},
		},
		units,
	)
	c.Assert(err, tc.ErrorIs, storageerrors.StorageInstanceNotFound)
}

// TestCreateIAASApplicationStorageInstanceNotAlive verifies that
// CreateIAASApplication fails when an expected storage instance is not alive.
func (s *applicationCreateStorageSuite) TestCreateIAASApplicationStorageInstanceNotAlive(c *tc.C) {
	platform := deployment.Platform{
		Channel:      "666",
		OSType:       deployment.Ubuntu,
		Architecture: architecture.ARM64,
	}
	channel := &deployment.Channel{
		Track:  "track",
		Risk:   "risk",
		Branch: "branch",
	}

	unitUUID := tc.Must(c, coreunit.NewUUID)
	netNodeUUID := tc.Must(c, domainnetwork.NewNetNodeUUID)
	machineUUID := tc.Must(c, coremachine.NewUUID)
	existingSIUUID := s.newStorageInstanceWithName(c, "st1")
	notAliveSIUUID := s.newStorageInstanceWithName(c, "st2")

	s.setStorageInstanceLife(c, notAliveSIUUID, domainlife.Dying)

	units := []application.AddIAASUnitArg{
		{
			AddUnitArg: application.AddUnitArg{
				CreateUnitStorageArg: domainstorage.CreateUnitStorageArg{
					ExistingStorageInstanceUUIDsToCheck: []domainstorage.StorageInstanceUUID{
						existingSIUUID,
						notAliveSIUUID,
					},
				},
				NetNodeUUID: netNodeUUID,
				UnitUUID:    unitUUID,
			},
			Platform:           platform,
			MachineNetNodeUUID: netNodeUUID,
			MachineUUID:        machineUUID,
		},
	}

	_, _, err := s.state.CreateIAASApplication(
		c.Context(),
		"app",
		application.AddIAASApplicationArg{
			BaseAddApplicationArg: application.BaseAddApplicationArg{
				Platform: platform,
				Charm: charm.Charm{
					Metadata:      s.minimalMetadata(c, "app"),
					Manifest:      s.minimalManifest(c),
					Source:        charm.CharmHubSource,
					ReferenceName: "app",
					Revision:      42,
					Architecture:  architecture.ARM64,
				},
				CharmDownloadInfo: &charm.DownloadInfo{
					Provenance:         charm.ProvenanceDownload,
					CharmhubIdentifier: "ident-1",
					DownloadURL:        "http://example.com/charm",
					DownloadSize:       666,
				},
				Channel: channel,
			},
		},
		units,
	)
	c.Assert(err, tc.ErrorIs, storageerrors.StorageInstanceNotAlive)
}

// TestCreateIAASApplicationStorageInstanceUnexpectedAttachments verifies that
// CreateIAASApplication fails when an expected storage instance has
// attachments but no expected attachments are supplied.
func (s *applicationCreateStorageSuite) TestCreateIAASApplicationStorageInstanceUnexpectedAttachments(c *tc.C) {
	platform := deployment.Platform{
		Channel:      "666",
		OSType:       deployment.Ubuntu,
		Architecture: architecture.ARM64,
	}
	channel := &deployment.Channel{
		Track:  "track",
		Risk:   "risk",
		Branch: "branch",
	}

	_, unitUUIDs := s.createIAASApplicationWithNUnits(
		c, "existing", domainlife.Alive, 1,
	)
	existingUnitUUID := unitUUIDs[0]

	storageInstUUID := s.newStorageInstanceWithName(c, "st1")
	s.newStorageInstanceAttachment(c, storageInstUUID, existingUnitUUID)

	unitUUID := tc.Must(c, coreunit.NewUUID)
	netNodeUUID := tc.Must(c, domainnetwork.NewNetNodeUUID)
	machineUUID := tc.Must(c, coremachine.NewUUID)

	units := []application.AddIAASUnitArg{
		{
			AddUnitArg: application.AddUnitArg{
				CreateUnitStorageArg: domainstorage.CreateUnitStorageArg{
					ExistingStorageInstanceUUIDsToCheck: []domainstorage.StorageInstanceUUID{
						storageInstUUID,
					},
					StorageInstanceAttachmentCheckArgs: []domainstorage.StorageInstanceAttachmentCheckArgs{
						{
							UUID: storageInstUUID,
						},
					},
				},
				NetNodeUUID: netNodeUUID,
				UnitUUID:    unitUUID,
			},
			Platform:           platform,
			MachineNetNodeUUID: netNodeUUID,
			MachineUUID:        machineUUID,
		},
	}

	_, _, err := s.state.CreateIAASApplication(
		c.Context(),
		"app",
		application.AddIAASApplicationArg{
			BaseAddApplicationArg: application.BaseAddApplicationArg{
				Platform: platform,
				Charm: charm.Charm{
					Metadata:      s.minimalMetadata(c, "app"),
					Manifest:      s.minimalManifest(c),
					Source:        charm.CharmHubSource,
					ReferenceName: "app",
					Revision:      42,
					Architecture:  architecture.ARM64,
				},
				CharmDownloadInfo: &charm.DownloadInfo{
					Provenance:         charm.ProvenanceDownload,
					CharmhubIdentifier: "ident-1",
					DownloadURL:        "http://example.com/charm",
					DownloadSize:       666,
				},
				Channel: channel,
			},
		},
		units,
	)
	c.Assert(err, tc.ErrorIs, applicationerrors.StorageInstanceUnexpectedAttachments)
}

// TestCreateIAASApplicationStorageInstanceUnexpectedAttachmentsExtra verifies
// that CreateIAASApplication fails when expected attachments are supplied but
// additional attachments exist.
func (s *applicationCreateStorageSuite) TestCreateIAASApplicationStorageInstanceUnexpectedAttachmentsExtra(c *tc.C) {
	platform := deployment.Platform{
		Channel:      "666",
		OSType:       deployment.Ubuntu,
		Architecture: architecture.ARM64,
	}
	channel := &deployment.Channel{
		Track:  "track",
		Risk:   "risk",
		Branch: "branch",
	}

	_, unitUUIDs := s.createIAASApplicationWithNUnits(
		c, "existing", domainlife.Alive, 2,
	)
	unitUUID1 := unitUUIDs[0]
	unitUUID2 := unitUUIDs[1]

	storageInstUUID := s.newStorageInstanceWithName(c, "st1")
	expectedAttachUUID := s.newStorageInstanceAttachment(c, storageInstUUID, unitUUID1)
	s.newStorageInstanceAttachment(c, storageInstUUID, unitUUID2)

	unitUUID := tc.Must(c, coreunit.NewUUID)
	netNodeUUID := tc.Must(c, domainnetwork.NewNetNodeUUID)
	machineUUID := tc.Must(c, coremachine.NewUUID)

	units := []application.AddIAASUnitArg{
		{
			AddUnitArg: application.AddUnitArg{
				CreateUnitStorageArg: domainstorage.CreateUnitStorageArg{
					ExistingStorageInstanceUUIDsToCheck: []domainstorage.StorageInstanceUUID{
						storageInstUUID,
					},
					StorageInstanceAttachmentCheckArgs: []domainstorage.StorageInstanceAttachmentCheckArgs{
						{
							ExpectedAttachments: []domainstorage.StorageAttachmentUUID{
								expectedAttachUUID,
							},
							UUID: storageInstUUID,
						},
					},
				},
				NetNodeUUID: netNodeUUID,
				UnitUUID:    unitUUID,
			},
			Platform:           platform,
			MachineNetNodeUUID: netNodeUUID,
			MachineUUID:        machineUUID,
		},
	}

	_, _, err := s.state.CreateIAASApplication(
		c.Context(),
		"app",
		application.AddIAASApplicationArg{
			BaseAddApplicationArg: application.BaseAddApplicationArg{
				Platform: platform,
				Charm: charm.Charm{
					Metadata:      s.minimalMetadata(c, "app"),
					Manifest:      s.minimalManifest(c),
					Source:        charm.CharmHubSource,
					ReferenceName: "app",
					Revision:      42,
					Architecture:  architecture.ARM64,
				},
				CharmDownloadInfo: &charm.DownloadInfo{
					Provenance:         charm.ProvenanceDownload,
					CharmhubIdentifier: "ident-1",
					DownloadURL:        "http://example.com/charm",
					DownloadSize:       666,
				},
				Channel: channel,
			},
		},
		units,
	)
	c.Assert(err, tc.ErrorIs, applicationerrors.StorageInstanceUnexpectedAttachments)
}

// TestCreateIAASApplicationWithExistingStorageAttachments verifies that
// CreateIAASApplication attaches multiple existing storage instances to the
// new unit.
func (s *applicationCreateStorageSuite) TestCreateIAASApplicationWithExistingStorageAttachments(c *tc.C) {
	platform := deployment.Platform{
		Channel:      "666",
		OSType:       deployment.Ubuntu,
		Architecture: architecture.ARM64,
	}
	channel := &deployment.Channel{
		Track:  "track",
		Risk:   "risk",
		Branch: "branch",
	}

	unitUUID := tc.Must(c, coreunit.NewUUID)
	netNodeUUID := tc.Must(c, domainnetwork.NewNetNodeUUID)
	machineUUID := tc.Must(c, coremachine.NewUUID)

	storageInst1UUID := s.newStorageInstanceWithName(c, "st1")
	storageInst2UUID := s.newStorageInstanceWithName(c, "st2")
	attach1UUID := tc.Must(c, domainstorage.NewStorageAttachmentUUID)
	attach2UUID := tc.Must(c, domainstorage.NewStorageAttachmentUUID)

	units := []application.AddIAASUnitArg{
		{
			AddUnitArg: application.AddUnitArg{
				CreateUnitStorageArg: domainstorage.CreateUnitStorageArg{
					ExistingStorageInstanceUUIDsToCheck: []domainstorage.StorageInstanceUUID{
						storageInst1UUID,
						storageInst2UUID,
					},
					StorageInstanceAttachmentCheckArgs: []domainstorage.StorageInstanceAttachmentCheckArgs{
						{UUID: storageInst1UUID},
						{UUID: storageInst2UUID},
					},
					StorageToAttach: []domainstorage.CreateUnitStorageAttachmentArg{
						{
							StorageInstanceUUID: storageInst1UUID,
							UUID:                attach1UUID,
						},
						{
							StorageInstanceUUID: storageInst2UUID,
							UUID:                attach2UUID,
						},
					},
				},
				NetNodeUUID: netNodeUUID,
				UnitUUID:    unitUUID,
			},
			Platform:           platform,
			MachineNetNodeUUID: netNodeUUID,
			MachineUUID:        machineUUID,
		},
	}

	_, _, err := s.state.CreateIAASApplication(
		c.Context(),
		"app",
		application.AddIAASApplicationArg{
			BaseAddApplicationArg: application.BaseAddApplicationArg{
				Platform: platform,
				Charm: charm.Charm{
					Metadata:      s.minimalMetadata(c, "app"),
					Manifest:      s.minimalManifest(c),
					Source:        charm.CharmHubSource,
					ReferenceName: "app",
					Revision:      42,
					Architecture:  architecture.ARM64,
				},
				CharmDownloadInfo: &charm.DownloadInfo{
					Provenance:         charm.ProvenanceDownload,
					CharmhubIdentifier: "ident-1",
					DownloadURL:        "http://example.com/charm",
					DownloadSize:       666,
				},
				Channel: channel,
			},
		},
		units,
	)
	c.Assert(err, tc.ErrorIsNil)

	s.assertStorageInstanceAttachmentExists(
		c,
		attach1UUID,
		storageInst1UUID,
		unitUUID,
	)
	s.assertStorageInstanceAttachmentExists(
		c,
		attach2UUID,
		storageInst2UUID,
		unitUUID,
	)
}

// TestCreateCAASApplicationStorageInstanceUnexpectedAttachments verifies that
// CreateCAASApplication fails when an expected storage instance has
// attachments but no expected attachments are supplied.
func (s *applicationCreateStorageSuite) TestCreateCAASApplicationStorageInstanceUnexpectedAttachments(c *tc.C) {
	platform := deployment.Platform{
		Channel:      "666",
		OSType:       deployment.Ubuntu,
		Architecture: architecture.ARM64,
	}
	channel := &deployment.Channel{
		Track:  "track",
		Risk:   "risk",
		Branch: "branch",
	}

	_, unitUUIDs := s.createIAASApplicationWithNUnits(
		c, "existing", domainlife.Alive, 1,
	)
	existingUnitUUID := unitUUIDs[0]

	storageInstUUID := s.newStorageInstanceWithName(c, "st1")
	s.newStorageInstanceAttachment(c, storageInstUUID, existingUnitUUID)

	unitUUID := tc.Must(c, coreunit.NewUUID)
	netNodeUUID := tc.Must(c, domainnetwork.NewNetNodeUUID)

	units := []application.AddCAASUnitArg{
		{
			AddUnitArg: application.AddUnitArg{
				CreateUnitStorageArg: domainstorage.CreateUnitStorageArg{
					ExistingStorageInstanceUUIDsToCheck: []domainstorage.StorageInstanceUUID{
						storageInstUUID,
					},
					StorageInstanceAttachmentCheckArgs: []domainstorage.StorageInstanceAttachmentCheckArgs{
						{
							UUID: storageInstUUID,
						},
					},
				},
				NetNodeUUID: netNodeUUID,
				UnitUUID:    unitUUID,
			},
		},
	}

	_, err := s.state.CreateCAASApplication(
		c.Context(),
		"app",
		application.AddCAASApplicationArg{
			BaseAddApplicationArg: application.BaseAddApplicationArg{
				Platform: platform,
				Charm: charm.Charm{
					Metadata:      s.minimalMetadata(c, "app"),
					Manifest:      s.minimalManifest(c),
					Source:        charm.CharmHubSource,
					ReferenceName: "app",
					Revision:      42,
					Architecture:  architecture.ARM64,
				},
				CharmDownloadInfo: &charm.DownloadInfo{
					Provenance:         charm.ProvenanceDownload,
					CharmhubIdentifier: "ident-1",
					DownloadURL:        "http://example.com/charm",
					DownloadSize:       666,
				},
				Channel: channel,
			},
			Scale: 1,
		},
		units,
	)
	c.Assert(err, tc.ErrorIs, applicationerrors.StorageInstanceUnexpectedAttachments)
}

// TestCreateCAASApplicationStorageInstanceUnexpectedAttachmentsExtra verifies
// that CreateCAASApplication fails when expected attachments are supplied but
// additional attachments exist.
func (s *applicationCreateStorageSuite) TestCreateCAASApplicationStorageInstanceUnexpectedAttachmentsExtra(c *tc.C) {
	platform := deployment.Platform{
		Channel:      "666",
		OSType:       deployment.Ubuntu,
		Architecture: architecture.ARM64,
	}
	channel := &deployment.Channel{
		Track:  "track",
		Risk:   "risk",
		Branch: "branch",
	}

	_, unitUUIDs := s.createIAASApplicationWithNUnits(
		c, "existing", domainlife.Alive, 2,
	)
	unitUUID1 := unitUUIDs[0]
	unitUUID2 := unitUUIDs[1]

	storageInstUUID := s.newStorageInstanceWithName(c, "st1")
	expectedAttachUUID := s.newStorageInstanceAttachment(c, storageInstUUID, unitUUID1)
	s.newStorageInstanceAttachment(c, storageInstUUID, unitUUID2)

	unitUUID := tc.Must(c, coreunit.NewUUID)
	netNodeUUID := tc.Must(c, domainnetwork.NewNetNodeUUID)

	units := []application.AddCAASUnitArg{
		{
			AddUnitArg: application.AddUnitArg{
				CreateUnitStorageArg: domainstorage.CreateUnitStorageArg{
					ExistingStorageInstanceUUIDsToCheck: []domainstorage.StorageInstanceUUID{
						storageInstUUID,
					},
					StorageInstanceAttachmentCheckArgs: []domainstorage.StorageInstanceAttachmentCheckArgs{
						{
							ExpectedAttachments: []domainstorage.StorageAttachmentUUID{
								expectedAttachUUID,
							},
							UUID: storageInstUUID,
						},
					},
				},
				NetNodeUUID: netNodeUUID,
				UnitUUID:    unitUUID,
			},
		},
	}

	_, err := s.state.CreateCAASApplication(
		c.Context(),
		"app",
		application.AddCAASApplicationArg{
			BaseAddApplicationArg: application.BaseAddApplicationArg{
				Platform: platform,
				Charm: charm.Charm{
					Metadata:      s.minimalMetadata(c, "app"),
					Manifest:      s.minimalManifest(c),
					Source:        charm.CharmHubSource,
					ReferenceName: "app",
					Revision:      42,
					Architecture:  architecture.ARM64,
				},
				CharmDownloadInfo: &charm.DownloadInfo{
					Provenance:         charm.ProvenanceDownload,
					CharmhubIdentifier: "ident-1",
					DownloadURL:        "http://example.com/charm",
					DownloadSize:       666,
				},
				Channel: channel,
			},
			Scale: 1,
		},
		units,
	)
	c.Assert(err, tc.ErrorIs, applicationerrors.StorageInstanceUnexpectedAttachments)
}

// TestCreateCAASApplicationStorageInstanceNotFound verifies that
// CreateCAASApplication fails when an expected storage instance UUID does not
// exist.
func (s *applicationCreateStorageSuite) TestCreateCAASApplicationStorageInstanceNotFound(c *tc.C) {
	platform := deployment.Platform{
		Channel:      "666",
		OSType:       deployment.Ubuntu,
		Architecture: architecture.ARM64,
	}
	channel := &deployment.Channel{
		Track:  "track",
		Risk:   "risk",
		Branch: "branch",
	}

	unitUUID := tc.Must(c, coreunit.NewUUID)
	netNodeUUID := tc.Must(c, domainnetwork.NewNetNodeUUID)
	existingSIUUID := s.newStorageInstanceWithName(c, "st1")
	notFoundSIUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)

	units := []application.AddCAASUnitArg{
		{
			AddUnitArg: application.AddUnitArg{
				CreateUnitStorageArg: domainstorage.CreateUnitStorageArg{
					ExistingStorageInstanceUUIDsToCheck: []domainstorage.StorageInstanceUUID{
						existingSIUUID,
						notFoundSIUUID,
					},
				},
				NetNodeUUID: netNodeUUID,
				UnitUUID:    unitUUID,
			},
		},
	}

	_, err := s.state.CreateCAASApplication(
		c.Context(),
		"app",
		application.AddCAASApplicationArg{
			BaseAddApplicationArg: application.BaseAddApplicationArg{
				Platform: platform,
				Charm: charm.Charm{
					Metadata:      s.minimalMetadata(c, "app"),
					Manifest:      s.minimalManifest(c),
					Source:        charm.CharmHubSource,
					ReferenceName: "app",
					Revision:      42,
					Architecture:  architecture.ARM64,
				},
				CharmDownloadInfo: &charm.DownloadInfo{
					Provenance:         charm.ProvenanceDownload,
					CharmhubIdentifier: "ident-1",
					DownloadURL:        "http://example.com/charm",
					DownloadSize:       666,
				},
				Channel: channel,
			},
			Scale: 1,
		},
		units,
	)
	c.Assert(err, tc.ErrorIs, storageerrors.StorageInstanceNotFound)
}

// TestCreateCAASApplicationStorageInstanceNotAlive verifies that
// CreateCAASApplication fails when an expected storage instance is not alive.
func (s *applicationCreateStorageSuite) TestCreateCAASApplicationStorageInstanceNotAlive(c *tc.C) {
	platform := deployment.Platform{
		Channel:      "666",
		OSType:       deployment.Ubuntu,
		Architecture: architecture.ARM64,
	}
	channel := &deployment.Channel{
		Track:  "track",
		Risk:   "risk",
		Branch: "branch",
	}

	unitUUID := tc.Must(c, coreunit.NewUUID)
	netNodeUUID := tc.Must(c, domainnetwork.NewNetNodeUUID)
	existingSIUUID := s.newStorageInstanceWithName(c, "st1")
	notAliveSIUUID := s.newStorageInstanceWithName(c, "st2")

	s.setStorageInstanceLife(c, notAliveSIUUID, domainlife.Dying)

	units := []application.AddCAASUnitArg{
		{
			AddUnitArg: application.AddUnitArg{
				CreateUnitStorageArg: domainstorage.CreateUnitStorageArg{
					ExistingStorageInstanceUUIDsToCheck: []domainstorage.StorageInstanceUUID{
						existingSIUUID,
						notAliveSIUUID,
					},
				},
				NetNodeUUID: netNodeUUID,
				UnitUUID:    unitUUID,
			},
		},
	}

	_, err := s.state.CreateCAASApplication(
		c.Context(),
		"app",
		application.AddCAASApplicationArg{
			BaseAddApplicationArg: application.BaseAddApplicationArg{
				Platform: platform,
				Charm: charm.Charm{
					Metadata:      s.minimalMetadata(c, "app"),
					Manifest:      s.minimalManifest(c),
					Source:        charm.CharmHubSource,
					ReferenceName: "app",
					Revision:      42,
					Architecture:  architecture.ARM64,
				},
				CharmDownloadInfo: &charm.DownloadInfo{
					Provenance:         charm.ProvenanceDownload,
					CharmhubIdentifier: "ident-1",
					DownloadURL:        "http://example.com/charm",
					DownloadSize:       666,
				},
				Channel: channel,
			},
			Scale: 1,
		},
		units,
	)
	c.Assert(err, tc.ErrorIs, storageerrors.StorageInstanceNotAlive)
}

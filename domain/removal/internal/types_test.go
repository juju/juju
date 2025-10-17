// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
)

type typesSuite struct {
	testhelpers.IsolationSuite
}

func TestTypesSuite(t *testing.T) {
	tc.Run(t, &typesSuite{})
}

func (s *typesSuite) TestCascadedStorageLivesEmpty(c *tc.C) {
	csl := CascadedStorageLives{}

	csl.StorageAttachmentUUIDs = nil
	csl.StorageInstanceUUIDs = []string{"burp"}
	c.Check(csl.IsEmpty(), tc.IsFalse)

	csl.StorageInstanceUUIDs = nil
	csl.FileSystemUUIDs = []string{"burp"}
	c.Check(csl.IsEmpty(), tc.IsFalse)

	csl.FileSystemUUIDs = nil
	csl.FileSystemAttachmentUUIDs = []string{"burp"}
	c.Check(csl.IsEmpty(), tc.IsFalse)

	csl.FileSystemAttachmentUUIDs = nil
	csl.VolumeUUIDs = []string{"burp"}
	c.Check(csl.IsEmpty(), tc.IsFalse)

	csl.VolumeUUIDs = nil
	csl.VolumeAttachmentUUIDs = []string{"burp"}
	c.Check(csl.IsEmpty(), tc.IsFalse)

	csl.VolumeAttachmentUUIDs = nil
	csl.VolumeAttachmentPlanUUIDs = []string{"burp"}
	c.Check(csl.IsEmpty(), tc.IsFalse)
}

func (s *typesSuite) TestCascadedStorageLivesMergeInstance(c *tc.C) {
	merged := CascadedStorageLives{
		StorageInstanceUUIDs: []string{"si1"},
	}.MergeInstance(CascadedStorageInstanceLives{
		FileSystemUUID:           ptr("fs1"),
		FileSystemAttachmentUUID: ptr("fsa1"),
	}).MergeInstance(CascadedStorageInstanceLives{
		FileSystemUUID:           ptr("fs2"),
		FileSystemAttachmentUUID: ptr("fsa2"),
	}).MergeInstance(CascadedStorageInstanceLives{
		VolumeUUID:               ptr("v1"),
		VolumeAttachmentUUID:     ptr("va1"),
		VolumeAttachmentPlanUUID: ptr("vap1"),
	})

	expected := CascadedStorageLives{
		StorageInstanceUUIDs:      []string{"si1"},
		StorageAttachmentUUIDs:    nil,
		FileSystemUUIDs:           []string{"fs1", "fs2"},
		FileSystemAttachmentUUIDs: []string{"fsa1", "fsa2"},
		VolumeUUIDs:               []string{"v1"},
		VolumeAttachmentUUIDs:     []string{"va1"},
		VolumeAttachmentPlanUUIDs: []string{"vap1"},
	}

	c.Check(merged, tc.DeepEquals, expected)
}

func (s *typesSuite) TestCascadedUnitLivesEmpty(c *tc.C) {
	cul := CascadedUnitLives{}
	c.Check(cul.IsEmpty(), tc.IsTrue)

	mUUID := "watever"
	cul.MachineUUID = &mUUID
	c.Check(cul.IsEmpty(), tc.IsFalse)

	cul.MachineUUID = nil
	cul.StorageAttachmentUUIDs = []string{"burp"}
	c.Check(cul.IsEmpty(), tc.IsFalse)

	cul.StorageAttachmentUUIDs = nil
	cul.StorageInstanceUUIDs = []string{"burp"}
	c.Check(cul.IsEmpty(), tc.IsFalse)
}

func (s *typesSuite) TestCascadedMachineLivesEmpty(c *tc.C) {
	cml := CascadedMachineLives{}
	c.Check(cml.IsEmpty(), tc.IsTrue)

	cml.MachineUUIDs = []string{"burp"}
	c.Check(cml.IsEmpty(), tc.IsFalse)

	cml.MachineUUIDs = nil
	cml.UnitUUIDs = []string{"burp"}
	c.Check(cml.IsEmpty(), tc.IsFalse)

	cml.UnitUUIDs = nil
	cml.StorageAttachmentUUIDs = []string{"burp"}
	c.Check(cml.IsEmpty(), tc.IsFalse)

}

func (s *typesSuite) TestCascadedApplicationLivesEmpty(c *tc.C) {
	cal := CascadedApplicationLives{}
	c.Check(cal.IsEmpty(), tc.IsTrue)

	cal.MachineUUIDs = []string{"burp"}
	c.Check(cal.IsEmpty(), tc.IsFalse)

	cal.MachineUUIDs = nil
	cal.UnitUUIDs = []string{"burp"}
	c.Check(cal.IsEmpty(), tc.IsFalse)

	cal.UnitUUIDs = nil
	cal.StorageAttachmentUUIDs = []string{"burp"}
	c.Check(cal.IsEmpty(), tc.IsFalse)

	cal.StorageAttachmentUUIDs = nil
	cal.RelationUUIDs = []string{"burp"}
	c.Check(cal.IsEmpty(), tc.IsFalse)
}

func ptr[T any](v T) *T {
	return &v
}

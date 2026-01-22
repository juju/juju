// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"testing"

	"github.com/juju/tc"
)

// volumeAttachmentPlanUUIDSuite is a suite of tests for asserting the behaviour
// of [volumeAttachmentPlanUUIDSuite].
type volumeAttachmentPlanUUIDSuite struct{}

// TestVolumeAttachmentPlanUUIDSuite runs all of the tests contained within
// [volumeAttachmentPlanUUIDSuite].
func TestVolumeAttachmentPlanUUIDSuite(t *testing.T) {
	tc.Run(t, volumeAttachmentPlanUUIDSuite{})
}

// TestNew tests that constructing a new VolumeAttachmentPlanUUID succeeds with
// no errors and the end result is valid.
func (volumeAttachmentPlanUUIDSuite) TestNew(c *tc.C) {
	u, err := NewVolumeAttachmentPlanUUID()
	c.Check(err, tc.ErrorIsNil)
	c.Check(u.Validate(), tc.ErrorIsNil)
}

// TestStringer asserts the [fmt.Stringer] interface of
// [VolumeAttachmentPlanUUID] by making sure the correct string representation
// of the uuid is returned to the caller.
func (volumeAttachmentPlanUUIDSuite) TestStringer(c *tc.C) {
	const validUUID = "0de7ed80-bfcf-49b1-876a-31462e940ca1"
	c.Check(VolumeAttachmentPlanUUID(validUUID).String(), tc.Equals, validUUID)
}

// TestValidate asserts that a valid uuid passes validation with no errors.
func (volumeAttachmentPlanUUIDSuite) TestValidate(c *tc.C) {
	const validUUID = "0de7ed80-bfcf-49b1-876a-31462e940ca1"
	c.Check(VolumeAttachmentPlanUUID(validUUID).Validate(), tc.ErrorIsNil)
}

// TestValidateFail asserts that a bad uuid fails validation.
func (volumeAttachmentPlanUUIDSuite) TestValidateFail(c *tc.C) {
	c.Check(VolumeAttachmentPlanUUID("invalid").Validate(), tc.NotNil)
}

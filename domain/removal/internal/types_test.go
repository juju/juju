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

	cml.StorageAttachmentUUIDs = nil
	cml.StorageInstanceUUIDs = []string{"burp"}
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
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

// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/domain/storageprovisioning"
	"github.com/juju/juju/internal/uuid"
)

// storageSuite is a test suite for asserting the behaviour of general
// methods on [State].
type storageSuite struct {
	baseSuite
}

func TestStorageSuite(t *testing.T) {
	tc.Run(t, &storageSuite{})
}

func (s *storageSuite) TestGetStorageResourceTagInfoForModel(c *tc.C) {
	controllerUUID := uuid.MustNewUUID().String()

	_, err := s.DB().ExecContext(
		c.Context(),
		"INSERT INTO model_config (key, value) VALUES (?, ?)",
		"resource_tags",
		"a=x b=y",
	)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().ExecContext(
		c.Context(),
		`
INSERT INTO model (uuid, controller_uuid, name, qualifier, type, cloud, cloud_type)
VALUES (?, ?, "", "", "", "", "")
`,
		s.ModelUUID(),
		controllerUUID,
	)
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory())
	resourceTags, err := st.GetStorageResourceTagInfoForModel(
		c.Context(), "resource_tags",
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(resourceTags, tc.DeepEquals, storageprovisioning.ModelResourceTagInfo{
		BaseResourceTags: "a=x b=y",
		ModelUUID:        s.ModelUUID(),
		ControllerUUID:   controllerUUID,
	})
}

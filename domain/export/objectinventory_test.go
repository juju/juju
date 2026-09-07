// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package export_test

import (
	stdtesting "testing"

	"github.com/juju/tc"

	domainexport "github.com/juju/juju/domain/export"
	ctrlv4_1_0 "github.com/juju/juju/domain/export/types/controller/v4_1_0"
	"github.com/juju/juju/domain/export/types/latest"
	v4_1_0 "github.com/juju/juju/domain/export/types/v4_1_0"
	"github.com/juju/juju/internal/testing"
)

type objectInventorySuite struct {
	testing.BaseSuite
}

func TestObjectInventorySuite(t *stdtesting.T) {
	tc.Run(t, &objectInventorySuite{})
}

func (s *objectInventorySuite) TestControllerObjectInventory(c *tc.C) {
	payload := &ctrlv4_1_0.ControllerExport{
		ObjectStoreMetadata: []ctrlv4_1_0.ObjectStoreMetadata{
			{UUID: "uuid-1", Sha256: "sha256-a", Sha384: "sha384-a", Size: 10},
			// Multiple logical paths referring to one object share the
			// object's metadata row shape; the inventory holds one entry
			// per distinct SHA-384.
			{UUID: "uuid-1", Sha256: "sha256-a", Sha384: "sha384-a", Size: 10},
			{UUID: "uuid-2", Sha256: "sha256-b", Sha384: "sha384-b", Size: 20},
		},
	}

	entries, err := domainexport.ControllerObjectInventory(domainexport.ControllerExport{
		Payload: payload,
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(entries, tc.DeepEquals, []domainexport.ObjectInventoryEntry{
		{Namespace: domainexport.ControllerObjectStoreNamespace, SHA256: "sha256-a", SHA384: "sha384-a", Size: 10},
		{Namespace: domainexport.ControllerObjectStoreNamespace, SHA256: "sha256-b", SHA384: "sha384-b", Size: 20},
	})
}

func (s *objectInventorySuite) TestModelObjectInventory(c *tc.C) {
	modelUUID := "deadbeef-0bad-400d-8000-4b1d0d06f00d"
	payload := &latest.ModelExport{
		ObjectStoreMetadata: []v4_1_0.ObjectStoreMetadata{
			{UUID: "uuid-1", Sha256: "sha256-a", Sha384: "sha384-a", Size: 10},
		},
	}

	entries, err := domainexport.ModelObjectInventory(domainexport.ModelExport{
		Payload: payload,
	}, modelUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(entries, tc.DeepEquals, []domainexport.ObjectInventoryEntry{
		{Namespace: modelUUID, SHA256: "sha256-a", SHA384: "sha384-a", Size: 10},
	})
}

func (s *objectInventorySuite) TestControllerObjectInventoryEmpty(c *tc.C) {
	entries, err := domainexport.ControllerObjectInventory(domainexport.ControllerExport{
		Payload: &ctrlv4_1_0.ControllerExport{},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(entries, tc.HasLen, 0)
}

func (s *objectInventorySuite) TestControllerObjectInventoryBadPayload(c *tc.C) {
	_, err := domainexport.ControllerObjectInventory(domainexport.ControllerExport{
		Payload: "not a controller payload",
	})
	c.Assert(err, tc.ErrorMatches, `unexpected controller export payload type string`)
}

func (s *objectInventorySuite) TestModelObjectInventoryBadPayload(c *tc.C) {
	_, err := domainexport.ModelObjectInventory(domainexport.ModelExport{
		Payload: "not a model payload",
	}, "some-model")
	c.Assert(err, tc.ErrorMatches, `unexpected model export payload type string`)
}

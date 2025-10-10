// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"fmt"
	"maps"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/juju/tc"

	coreerrors "github.com/juju/juju/core/errors"
)

type storagePoolSuite struct {
}

var knownDefaultProviderPools = []struct {
	Name         string
	ProviderType string
}{
	{Name: "azure", ProviderType: "azure"},
	{Name: "azure-premium", ProviderType: "azure"},
	{Name: "cinder", ProviderType: "cinder"},
	{Name: "ebs", ProviderType: "ebs"},
	{Name: "ebs-ssd", ProviderType: "ebs"},
	{Name: "gce", ProviderType: "gce"},
	{Name: "kubernetes", ProviderType: "kubernetes"},
	{Name: "loop", ProviderType: "loop"},
	{Name: "lxd", ProviderType: "lxd"},
	{Name: "lxd-btrfs", ProviderType: "lxd"},
	{Name: "lxd-zfs", ProviderType: "lxd"},
	{Name: "maas", ProviderType: "maas"},
	{Name: "oci", ProviderType: "oci"},
	{Name: "iscsi", ProviderType: "oci"},
	{Name: "rootfs", ProviderType: "rootfs"},
	{Name: "tmpfs", ProviderType: "tmpfs"},
}

func TestStoragePoolSuite(t *testing.T) {
	tc.Run(t, storagePoolSuite{})
}

// TestDefaultStoragePoolSkew tests that this test has the same number of
// default storage pools as that of the domain. If this test fails it means they
// need to be updated to include new additions or breaking changes have been
// made.
func (storagePoolSuite) TestDefaultStoragePoolSkew(c *tc.C) {
	c.Assert(getDefaultStoragePoolUUIDs(), tc.HasLen, len(knownDefaultProviderPools))
}

// TestDefaultProviderPoolUUIDs tests each of the default storage provider uuids
// to make sure that is is constructed from the uuid namespace with the pool
// name and provider type.
func (storagePoolSuite) TestDefaultProviderPoolUUIDs(c *tc.C) {
	jujuUUIDNamespace := "96bb15e6-8b85-448b-9fce-ede1a1700e64"
	namespaceUUID, err := uuid.Parse(jujuUUIDNamespace)
	c.Assert(err, tc.ErrorIsNil)

	for _, pool := range knownDefaultProviderPools {
		c.Run(pool.ProviderType+"-"+pool.Name, func(t *testing.T) {
			poolDomain := fmt.Sprintf("juju.storage.pool.%s.%s", pool.ProviderType, pool.Name)
			expectedUUID := uuid.NewSHA1(namespaceUUID, []byte(poolDomain))

			defUUID, err := GetProviderDefaultStoragePoolUUID(pool.Name, pool.ProviderType)
			tc.Assert(t, err, tc.ErrorIsNil)
			tc.Check(t, defUUID.String(), tc.Equals, expectedUUID.String())
		})
	}
}

func (storagePoolSuite) TestGetProviderDefaultStoragePoolUUIDNotFound(c *tc.C) {
	_, err := GetProviderDefaultStoragePoolUUID("noexistpool", "phonyprovider")
	c.Check(err, tc.ErrorIs, coreerrors.NotFound)
}

// TestGetProviderDefaultStoragePoolUUIDOrMakeExisting tests that for a pool and
// provider that is considered a default no new uuid is created.
func (storagePoolSuite) TestGetProviderDefaultStoragePoolUUIDOrMakeExisting(
	c *tc.C,
) {
	uuid, err := GenerateProviderDefaultStoragePoolUUIDWithDefaults("loop", "loop")
	c.Check(err, tc.ErrorIsNil)
	c.Check(uuid.String(), tc.Equals, "baa26e04-b1f0-50d9-9bf8-4d5a78ffe6ad")
}

func (storagePoolSuite) TestGetProviderDefaultStoragePoolUUIDOrMakeNew(
	c *tc.C,
) {
	uuid, err := GenerateProviderDefaultStoragePoolUUIDWithDefaults("foo", "bar")
	c.Check(err, tc.ErrorIsNil)
	c.Check(uuid.String(), tc.IsNonZeroUUID)

	defaultUUIDs := maps.Values(getDefaultStoragePoolUUIDs())
	exists := slices.Contains(slices.Collect(defaultUUIDs), uuid)
	c.Check(exists, tc.IsFalse)
}

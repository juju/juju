// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"strings"
	"testing"

	"github.com/juju/tc"

	domainstorage "github.com/juju/juju/domain/storage"
	"github.com/juju/juju/internal/provider/kubernetes/constants"
	"github.com/juju/juju/internal/storage"
)

type kubernetesSuite struct{}

func TestKubernetes(t *testing.T) {
	tc.Run(t, &kubernetesSuite{})
}

func (s *kubernetesSuite) TestPoolNameForStorageClass(c *tc.C) {
	for _, test := range []struct {
		className string
		poolName  string
	}{
		{className: "", poolName: ""},
		{className: "ceph-rbd", poolName: "ceph-rbd"},
		{className: "fast.ssd", poolName: "fast-ssd"},
		{className: "0-fast", poolName: "k8s-0-fast"},
		{className: "0", poolName: "k8s-0"},
		{className: "a", poolName: "a"},
		{className: "...", poolName: "k8s"},
		{className: "kubernetes", poolName: "k8s-kubernetes"},
		{className: "rootfs", poolName: "k8s-rootfs"},
		{className: "tmpfs", poolName: "k8s-tmpfs"},
		{
			// A name that truncates to a reserved pool name is prefixed.
			className: "rootfs" + strings.Repeat("-", 122) + "a",
			poolName:  "k8s-rootfs",
		},
		{
			className: strings.Repeat("a", 63) + "." + strings.Repeat("b", 63) +
				"." + strings.Repeat("c", 63) + "." + strings.Repeat("d", 61),
			poolName: strings.Repeat("a", 63) + "-" + strings.Repeat("b", 63),
		},
		{
			className: "0." + strings.Repeat("a", 63) + "." + strings.Repeat("b", 63),
			poolName:  "k8s-0-" + strings.Repeat("a", 63) + "-" + strings.Repeat("b", 58),
		},
	} {
		c.Run(test.className, func(t *testing.T) {
			c := &tc.TBC{TB: t}
			poolName := storage.PoolNameForStorageClass(test.className)
			c.Check(poolName, tc.Equals, test.poolName)
			if test.poolName != "" {
				// The output is always a valid Juju pool name, even for
				// class names that are not valid Kubernetes ones.
				c.Check(domainstorage.IsValidStoragePoolName(poolName), tc.IsTrue)
			}
		})
	}
}

func (s *kubernetesSuite) TestPoolNameForStorageClassTruncationLimit(c *tc.C) {
	// The derived name never exceeds the Juju pool name limit. The limit
	// is owned by the domain package, which depends on this one, so the
	// length is asserted against it to fail loudly on drift.
	poolName := storage.PoolNameForStorageClass(strings.Repeat("a", 500))
	c.Check(len(poolName), tc.Equals, domainstorage.StoragePoolNameMaxLength)
}

func (s *kubernetesSuite) TestSeededPoolNamesMatchProviderConstants(c *tc.C) {
	// The seeded pool names this package hardcodes must stay equal to the
	// Kubernetes provider types, whose constants package depends on this
	// one and so cannot be imported here.
	c.Check(string(constants.StorageProviderType), tc.Equals, "kubernetes")
	c.Check(string(constants.StorageProviderTypeRootfs), tc.Equals, "rootfs")
	c.Check(string(constants.StorageProviderTypeTmpfs), tc.Equals, "tmpfs")

	// A class named after a seeded pool never shadows it.
	c.Check(storage.PoolNameForStorageClass(string(constants.StorageProviderType)),
		tc.Equals, "k8s-kubernetes")
}

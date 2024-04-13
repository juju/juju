// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	gc "gopkg.in/check.v1"
	storagev1 "k8s.io/api/storage/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/juju/juju/internal/provider/caas/kubernetes/provider/storage"
)

type metadataSuite struct{}

var _ = gc.Suite(&metadataSuite{})

func (*metadataSuite) TestPreferredStorageAny(c *gc.C) {
	tests := []struct {
		Name         string
		StorageClass *storagev1.StorageClass
		Result       bool
	}{
		{
			Name: "Test Any Storage Class",
			StorageClass: &storagev1.StorageClass{
				ObjectMeta: meta.ObjectMeta{
					Name: "test1",
				},
			},
			Result: true,
		},
		{
			Name:         "Test Any Storage Class returns false for nil",
			StorageClass: nil,
			Result:       false,
		},
	}

	for _, test := range tests {
		c.Logf("running test %s", test.Name)
		any := storage.PreferredStorageAny{}
		c.Assert(any.Matches(test.StorageClass), gc.Equals, test.Result)
	}
}

func (*metadataSuite) TestPreferredStorageNominated(c *gc.C) {
	tests := []struct {
		Name             string
		StorageClass     *storagev1.StorageClass
		NominatedStorage string
		Result           bool
	}{
		{
			Name: "Test match nominated storage",
			StorageClass: &storagev1.StorageClass{
				ObjectMeta: meta.ObjectMeta{
					Name: "Test1",
				},
			},
			NominatedStorage: "Test1",
			Result:           true,
		},
		{
			Name:             "Test match nominated storage nil class",
			StorageClass:     nil,
			NominatedStorage: "test2",
			Result:           false,
		},
		{
			Name: "Test empty string does not match",
			StorageClass: &storagev1.StorageClass{
				ObjectMeta: meta.ObjectMeta{
					Name: "",
				},
			},
			NominatedStorage: "",
			Result:           false,
		},
		{
			Name: "Test case sensitive does not match",
			StorageClass: &storagev1.StorageClass{
				ObjectMeta: meta.ObjectMeta{
					Name: "AaBb",
				},
			},
			NominatedStorage: "aabb",
			Result:           false,
		},
	}

	for _, test := range tests {
		c.Logf("running test %s", test.Name)
		nominated := storage.PreferredStorageNominated{StorageClassName: test.NominatedStorage}
		c.Assert(nominated.Matches(test.StorageClass), gc.Equals, test.Result)
	}
}

func (*metadataSuite) TestPreferredStorageOperatorAnnotation(c *gc.C) {
	tests := []struct {
		Name         string
		StorageClass *storagev1.StorageClass
		Result       bool
	}{
		{
			Name: "Test operator storage annotation matches",
			StorageClass: &storagev1.StorageClass{
				ObjectMeta: meta.ObjectMeta{
					Name: "test1",
					Annotations: map[string]string{
						"juju.is/operator-storage": "true",
					},
				},
			},
			Result: true,
		},
		{
			Name: "Test operator storage doesn't match bad value",
			StorageClass: &storagev1.StorageClass{
				ObjectMeta: meta.ObjectMeta{
					Name: "test1",
					Annotations: map[string]string{
						"juju.is/operator-storage": "false",
					},
				},
			},
			Result: false,
		},
		{
			Name: "Test operator storage doesn't match workload storage",
			StorageClass: &storagev1.StorageClass{
				ObjectMeta: meta.ObjectMeta{
					Name: "test1",
					Annotations: map[string]string{
						"juju.is/workload-storage": "true",
					},
				},
			},
			Result: false,
		},
	}

	for _, test := range tests {
		c.Logf("running test %s", test.Name)
		annotation := storage.PreferredStorageOperatorAnnotation{}
		c.Assert(annotation.Matches(test.StorageClass), gc.Equals, test.Result)
	}
}

func (*metadataSuite) TestPreferredStorageWorkloadAnnotation(c *gc.C) {
	tests := []struct {
		Name         string
		StorageClass *storagev1.StorageClass
		Result       bool
	}{
		{
			Name: "Test operator storage annotation matches",
			StorageClass: &storagev1.StorageClass{
				ObjectMeta: meta.ObjectMeta{
					Name: "test1",
					Annotations: map[string]string{
						"juju.is/workload-storage": "true",
					},
				},
			},
			Result: true,
		},
		{
			Name: "Test operator storage doesn't match bad value",
			StorageClass: &storagev1.StorageClass{
				ObjectMeta: meta.ObjectMeta{
					Name: "test1",
					Annotations: map[string]string{
						"juju.is/workload-storage": "false",
					},
				},
			},
			Result: false,
		},
		{
			Name: "Test operator storage doesn't match operator storage",
			StorageClass: &storagev1.StorageClass{
				ObjectMeta: meta.ObjectMeta{
					Name: "test1",
					Annotations: map[string]string{
						"juju.is/operator-storage": "true",
					},
				},
			},
			Result: false,
		},
	}

	for _, test := range tests {
		c.Logf("running test %s", test.Name)
		annotation := storage.PreferredStorageWorkloadAnnotation{}
		c.Assert(annotation.Matches(test.StorageClass), gc.Equals, test.Result)
	}
}

func (*metadataSuite) TestPreferredStorageDefault(c *gc.C) {
	tests := []struct {
		Name         string
		StorageClass *storagev1.StorageClass
		Result       bool
	}{
		{
			Name: "Test default storage matches",
			StorageClass: &storagev1.StorageClass{
				ObjectMeta: meta.ObjectMeta{
					Name: "test1",
					Annotations: map[string]string{
						"storageclass.kubernetes.io/is-default-class": "true",
					},
				},
			},
			Result: true,
		},
		{
			Name: "Test default storage beta matches",
			StorageClass: &storagev1.StorageClass{
				ObjectMeta: meta.ObjectMeta{
					Name: "test1",
					Annotations: map[string]string{
						"storageclass.beta.kubernetes.io/is-default-class": "true",
					},
				},
			},
			Result: true,
		},
		{
			Name: "Test default storage both annotations match",
			StorageClass: &storagev1.StorageClass{
				ObjectMeta: meta.ObjectMeta{
					Name: "test1",
					Annotations: map[string]string{
						"storageclass.beta.kubernetes.io/is-default-class": "true",
						"storageclass.kubernetes.io/is-default-class":      "true",
					},
				},
			},
			Result: true,
		},
		{
			Name: "Test default storage both annotations different order match",
			StorageClass: &storagev1.StorageClass{
				ObjectMeta: meta.ObjectMeta{
					Name: "test1",
					Annotations: map[string]string{
						"storageclass.kubernetes.io/is-default-class":      "true",
						"storageclass.beta.kubernetes.io/is-default-class": "true",
					},
				},
			},
			Result: true,
		},
		{
			Name: "Test default storage type sensitive annotation",
			StorageClass: &storagev1.StorageClass{
				ObjectMeta: meta.ObjectMeta{
					Name: "test1",
					Annotations: map[string]string{
						"Storageclass.kubernetes.io/is-default-class": "true",
					},
				},
			},
			Result: false,
		},
		{
			Name: "Test default storage doesn't match",
			StorageClass: &storagev1.StorageClass{
				ObjectMeta: meta.ObjectMeta{
					Name: "test1",
					Annotations: map[string]string{
						"junk": "true",
					},
				},
			},
			Result: false,
		},
	}

	for _, test := range tests {
		c.Logf("running test %s", test.Name)
		defStorage := storage.PreferredStorageDefault{}
		c.Assert(defStorage.Matches(test.StorageClass), gc.Equals, test.Result)
	}
}

func (*metadataSuite) TestPreferredStorageProvisioner(c *gc.C) {
	tests := []struct {
		Name         string
		StorageClass *storagev1.StorageClass
		Provisioner  string
		Result       bool
	}{
		{
			Name: "Test provisioner empty string matches",
			StorageClass: &storagev1.StorageClass{
				ObjectMeta: meta.ObjectMeta{
					Name: "test1",
				},
				Provisioner: "",
			},
			Provisioner: "",
			Result:      true,
		},
		{
			Name: "Test Azure provisioner",
			StorageClass: &storagev1.StorageClass{
				ObjectMeta: meta.ObjectMeta{
					Name: "test1",
				},
				Provisioner: "kubernetes.io/azure-disk",
			},
			Provisioner: "kubernetes.io/azure-disk",
			Result:      true,
		},
		{
			Name: "Test provisioner doesn't match 1",
			StorageClass: &storagev1.StorageClass{
				ObjectMeta: meta.ObjectMeta{
					Name: "test1",
				},
				Provisioner: "kubernetes.io/azure-disk",
			},
			Provisioner: "",
			Result:      false,
		},
		{
			Name: "Test provisioner doesn't match 2",
			StorageClass: &storagev1.StorageClass{
				ObjectMeta: meta.ObjectMeta{
					Name: "test1",
				},
				Provisioner: "kubernetes.io/azure-disk",
			},
			Provisioner: "junk",
			Result:      false,
		},
	}

	for _, test := range tests {
		c.Logf("running test %s", test.Name)
		provisioner := storage.PreferredStorageProvisioner{
			NameVal:     "test-storage-provisioner",
			Provisioner: test.Provisioner,
		}
		c.Assert(provisioner.Matches(test.StorageClass), gc.Equals, test.Result)
	}
}

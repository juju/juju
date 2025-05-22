// Copyright 2019 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package charm

import (
	"testing"

	"github.com/juju/tc"
)

type removeRelationsSuite struct{}

func TestRemoveRelationsSuite(t *testing.T) {
	tc.Run(t, &removeRelationsSuite{})
}

var (
	sampleRelations = [][]string{
		{"kubernetes-master:kube-control", "kubernetes-worker:kube-control"},
		{"kubernetes-master:etcd", "etcd:db"},
		{"kubernetes-worker:kube-api-endpoint", "kubeapi-load-balancer:website"},
		{"flannel", "etcd"}, // removed :endpoint
		{"flannel:cni", "kubernetes-master:cni"},
		{"flannel:cni", "kubernetes-worker:cni"},
	}
)

func (*removeRelationsSuite) TestNil(c *tc.C) {
	result := removeRelations(nil, "foo")
	c.Assert(result, tc.HasLen, 0)
}

func (*removeRelationsSuite) TestEmpty(c *tc.C) {
	result := removeRelations([][]string{}, "foo")
	c.Assert(result, tc.HasLen, 0)
}

func (*removeRelationsSuite) TestAppNotThere(c *tc.C) {
	result := removeRelations(sampleRelations, "foo")
	c.Assert(result, tc.DeepEquals, sampleRelations)
}

func (*removeRelationsSuite) TestAppBadRelationsKept(c *tc.C) {
	badRelations := [][]string{{"single value"}, {"three", "string", "values"}}
	result := removeRelations(badRelations, "foo")
	c.Assert(result, tc.DeepEquals, badRelations)
}

func (*removeRelationsSuite) TestRemoveFromRight(c *tc.C) {
	result := removeRelations(sampleRelations, "etcd")
	c.Assert(result, tc.DeepEquals, [][]string{
		{"kubernetes-master:kube-control", "kubernetes-worker:kube-control"},
		{"kubernetes-worker:kube-api-endpoint", "kubeapi-load-balancer:website"},
		{"flannel:cni", "kubernetes-master:cni"},
		{"flannel:cni", "kubernetes-worker:cni"},
	})
}

func (*removeRelationsSuite) TestRemoveFromLeft(c *tc.C) {
	result := removeRelations(sampleRelations, "flannel")
	c.Assert(result, tc.DeepEquals, [][]string{
		{"kubernetes-master:kube-control", "kubernetes-worker:kube-control"},
		{"kubernetes-master:etcd", "etcd:db"},
		{"kubernetes-worker:kube-api-endpoint", "kubeapi-load-balancer:website"},
	})
}

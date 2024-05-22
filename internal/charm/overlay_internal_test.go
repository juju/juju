// Copyright 2019 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package charm

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type removeRelationsSuite struct{}

var (
	_ = gc.Suite(&removeRelationsSuite{})

	sampleRelations = [][]string{
		{"kubernetes-master:kube-control", "kubernetes-worker:kube-control"},
		{"kubernetes-master:etcd", "etcd:db"},
		{"kubernetes-worker:kube-api-endpoint", "kubeapi-load-balancer:website"},
		{"flannel", "etcd"}, // removed :endpoint
		{"flannel:cni", "kubernetes-master:cni"},
		{"flannel:cni", "kubernetes-worker:cni"},
	}
)

func (*removeRelationsSuite) TestNil(c *gc.C) {
	result := removeRelations(nil, "foo")
	c.Assert(result, gc.HasLen, 0)
}

func (*removeRelationsSuite) TestEmpty(c *gc.C) {
	result := removeRelations([][]string{}, "foo")
	c.Assert(result, gc.HasLen, 0)
}

func (*removeRelationsSuite) TestAppNotThere(c *gc.C) {
	result := removeRelations(sampleRelations, "foo")
	c.Assert(result, jc.DeepEquals, sampleRelations)
}

func (*removeRelationsSuite) TestAppBadRelationsKept(c *gc.C) {
	badRelations := [][]string{{"single value"}, {"three", "string", "values"}}
	result := removeRelations(badRelations, "foo")
	c.Assert(result, jc.DeepEquals, badRelations)
}

func (*removeRelationsSuite) TestRemoveFromRight(c *gc.C) {
	result := removeRelations(sampleRelations, "etcd")
	c.Assert(result, jc.DeepEquals, [][]string{
		{"kubernetes-master:kube-control", "kubernetes-worker:kube-control"},
		{"kubernetes-worker:kube-api-endpoint", "kubeapi-load-balancer:website"},
		{"flannel:cni", "kubernetes-master:cni"},
		{"flannel:cni", "kubernetes-worker:cni"},
	})
}

func (*removeRelationsSuite) TestRemoveFromLeft(c *gc.C) {
	result := removeRelations(sampleRelations, "flannel")
	c.Assert(result, jc.DeepEquals, [][]string{
		{"kubernetes-master:kube-control", "kubernetes-worker:kube-control"},
		{"kubernetes-master:etcd", "etcd:db"},
		{"kubernetes-worker:kube-api-endpoint", "kubeapi-load-balancer:website"},
	})
}

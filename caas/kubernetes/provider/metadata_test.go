// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	"github.com/juju/juju/caas/kubernetes/provider"
	gc "gopkg.in/check.v1"
	core "k8s.io/api/core/v1"
)

func newNode(labels map[string]string) core.Node {
	n := core.Node{}
	n.SetLabels(labels)
	return n
}

var nodesTestCases = []struct {
	expectedOut string
	node        core.Node
}{
	{
		expectedOut: "",
		node:        newNode(map[string]string{}),
	},
	{
		expectedOut: "",
		node: newNode(map[string]string{
			"cloud.google.com/gke-nodepool": "",
		}),
	},
	{
		expectedOut: "",
		node: newNode(map[string]string{
			"cloud.google.com/gke-os-distribution": "",
		}),
	},
	{
		expectedOut: "gce",
		node: newNode(map[string]string{
			"cloud.google.com/gke-nodepool":        "",
			"cloud.google.com/gke-os-distribution": "",
		}),
	},
	{
		expectedOut: "azure",
		node: newNode(map[string]string{
			"kubernetes.azure.com/cluster": "",
		}),
	},
	{
		expectedOut: "ec2",
		node: newNode(map[string]string{
			"manufacturer": "amazon_ec2",
		}),
	},
}

func (s *K8sSuite) TestGetCloudProviderFromNodeMeta(c *gc.C) {
	for _, v := range nodesTestCases {
		c.Check(provider.GetCloudProviderFromNodeMeta(v.node), gc.Equals, v.expectedOut)
	}
}

func (s *K8sSuite) TestK8sCloudCheckersValidationPass(c *gc.C) {
	// CompileK8sCloudCheckers will panic if there is invalid requirement definition so check it by calling it.
	cloudCheckers := provider.CompileK8sCloudCheckers()
	c.Assert(cloudCheckers, gc.NotNil)
}

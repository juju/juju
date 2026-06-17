// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	"context"
	"errors"
	"maps"
	"testing"

	"github.com/juju/tc"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsfake "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/juju/juju/core/network"
)

type k8sNetworkingSuite struct {
}

func TestK8sNetworkingSuite(t *testing.T) {
	tc.Run(t, &k8sNetworkingSuite{})
}

// fallbackResult is the catch-all subnet info returned when discovery finds
// nothing or is denied access.
var fallbackResult = []network.SubnetInfo{
	{CIDR: "0.0.0.0/0"},
	{CIDR: "::/0"},
}

// registeredCRs lists every CNI custom resource GVR the tests register with the
// fake dynamic client's scheme, so listing an empty/installed resource never
// raises a no-kind-registered error (CRD presence is gated via apiextensions).
var registeredCRs = []struct {
	gvr  schema.GroupVersionResource
	kind string
}{
	{schema.GroupVersionResource{Group: "crd.projectcalico.org", Version: "v1", Resource: "ippools"}, "IPPool"},
	{schema.GroupVersionResource{Group: "projectcalico.org", Version: "v3", Resource: "ippools"}, "IPPool"},
	{schema.GroupVersionResource{Group: "cilium.io", Version: "v2", Resource: "ciliumnodes"}, "CiliumNode"},
	{schema.GroupVersionResource{Group: "cilium.io", Version: "v2alpha1", Resource: "ciliumpodippools"}, "CiliumPodIPPool"},
	{schema.GroupVersionResource{Group: "config.openshift.io", Version: "v1", Resource: "networks"}, "Network"},
	{schema.GroupVersionResource{Group: "kubeovn.io", Version: "v1", Resource: "subnets"}, "Subnet"},
}

// clusterBuilder assembles fake clients seeded with core objects, CNI custom
// resources and CRD definitions.
type clusterBuilder struct {
	coreObjects    []k8sruntime.Object
	dynamicObjects []k8sruntime.Object
	crds           []k8sruntime.Object
}

func (b *clusterBuilder) addCore(objs ...k8sruntime.Object) *clusterBuilder {
	b.coreObjects = append(b.coreObjects, objs...)
	return b
}

func (b *clusterBuilder) addCR(objs ...*unstructured.Unstructured) *clusterBuilder {
	for _, o := range objs {
		b.dynamicObjects = append(b.dynamicObjects, o)
	}
	return b
}

func (b *clusterBuilder) addCRD(plural, group string, servedVersions ...string) *clusterBuilder {
	var versions []apiextensionsv1.CustomResourceDefinitionVersion
	for _, v := range servedVersions {
		versions = append(versions, apiextensionsv1.CustomResourceDefinitionVersion{Name: v, Served: true})
	}
	b.crds = append(b.crds, &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: plural + "." + group},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group:    group,
			Names:    apiextensionsv1.CustomResourceDefinitionNames{Plural: plural},
			Versions: versions,
		},
	})
	return b
}

func (b *clusterBuilder) build() (*fake.Clientset, *apiextensionsfake.Clientset, *dynamicfake.FakeDynamicClient) {
	typed := fake.NewClientset(b.coreObjects...)
	apiext := apiextensionsfake.NewSimpleClientset(b.crds...)

	scheme := k8sruntime.NewScheme()
	listKinds := map[schema.GroupVersionResource]string{}
	for _, r := range registeredCRs {
		gv := r.gvr.GroupVersion()
		scheme.AddKnownTypeWithName(gv.WithKind(r.kind), &unstructured.Unstructured{})
		scheme.AddKnownTypeWithName(gv.WithKind(r.kind+"List"), &unstructured.UnstructuredList{})
		listKinds[r.gvr] = r.kind + "List"
	}
	dyn := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, b.dynamicObjects...)
	return typed, apiext, dyn
}

func (b *clusterBuilder) networking() *environNetworking {
	typed, apiext, dyn := b.build()
	en := newEnvironNetworking(typed, apiext, dyn)
	return &en
}

func newClusterBuilder() *clusterBuilder {
	return &clusterBuilder{}
}

func node(name string, podCIDRs []string, annotations map[string]string) *corev1.Node {
	n := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name, Annotations: annotations},
	}
	if len(podCIDRs) > 0 {
		n.Spec.PodCIDR = podCIDRs[0]
		n.Spec.PodCIDRs = podCIDRs
	}
	return n
}

func configMap(namespace, name string, data map[string]string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
		Data:       data,
	}
}

func cr(apiVersion, kind, name string, spec map[string]any) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": apiVersion,
		"kind":       kind,
		"metadata":   map[string]any{"name": name},
		"spec":       spec,
	}}
}

func subnetInfo(cidrs ...string) []network.SubnetInfo {
	out := make([]network.SubnetInfo, len(cidrs))
	for i, c := range cidrs {
		out[i] = network.SubnetInfo{CIDR: c, ProviderId: network.Id(c)}
	}
	return out
}

func forbidden(resource string) error {
	return k8serrors.NewForbidden(schema.GroupResource{Resource: resource}, "", errors.New("nope"))
}

// -----------------------------------------------------------------------------
// Basic / generic behaviour
// -----------------------------------------------------------------------------

func (s *k8sNetworkingSuite) TestSupportsSpaces(c *tc.C) {
	envNet := &environNetworking{}

	ok, err := envNet.SupportsSpaces()

	c.Assert(err, tc.ErrorIsNil)
	c.Check(ok, tc.IsFalse)
}

func (s *k8sNetworkingSuite) TestSubnetsNilClientFallsBack(c *tc.C) {
	envNet := &environNetworking{}

	result, err := envNet.Subnets(c.Context(), nil)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, fallbackResult)
}

func (s *k8sNetworkingSuite) TestSubnetsNodePodCIDRs(c *tc.C) {
	envNet := newClusterBuilder().addCore(
		node("a", []string{"10.10.0.0/24"}, nil),
		node("b", []string{"fd10::/64", "10.10.1.0/24"}, nil),
		node("c", []string{"10.10.0.0/24", "not-a-cidr"}, nil),
	).networking()

	result, err := envNet.Subnets(c.Context(), nil)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, subnetInfo("10.10.0.0/24", "10.10.1.0/24", "fd10::/64"))
}

func (s *k8sNetworkingSuite) TestSubnetsNodeListErrorFallsBack(c *tc.C) {
	typed, apiext, dyn := newClusterBuilder().build()
	typed.PrependReactor("list", "nodes", func(k8stesting.Action) (bool, k8sruntime.Object, error) {
		return true, nil, errors.New("boom")
	})
	en := newEnvironNetworking(typed, apiext, dyn)

	result, err := en.Subnets(c.Context(), nil)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, fallbackResult)
}

// -----------------------------------------------------------------------------
// kube-router (node link)
// -----------------------------------------------------------------------------

func (s *k8sNetworkingSuite) TestKubeRouterPluralAnnotation(c *tc.C) {
	envNet := newClusterBuilder().addCore(
		node("a", nil, map[string]string{"kube-router.io/pod-cidrs": "10.20.0.0/24,fd20::/64"}),
	).networking()

	result, err := envNet.Subnets(c.Context(), nil)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, subnetInfo("10.20.0.0/24", "fd20::/64"))
}

func (s *k8sNetworkingSuite) TestKubeRouterLegacySingularAnnotation(c *tc.C) {
	envNet := newClusterBuilder().addCore(
		node("a", nil, map[string]string{"kube-router.io/pod-cidr": "10.21.0.0/24"}),
	).networking()

	result, err := envNet.Subnets(c.Context(), nil)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, subnetInfo("10.21.0.0/24"))
}

// -----------------------------------------------------------------------------
// Calico
// -----------------------------------------------------------------------------

func calicoIPPool(apiVersion, name, cidr string, extra map[string]any) *unstructured.Unstructured {
	spec := map[string]any{"cidr": cidr}
	maps.Copy(spec, extra)
	return cr(apiVersion, "IPPool", name, spec)
}

func (s *k8sNetworkingSuite) TestCalicoIPPoolV1(c *tc.C) {
	envNet := newClusterBuilder().
		addCRD("ippools", "crd.projectcalico.org", "v1").
		addCR(calicoIPPool("crd.projectcalico.org/v1", "default-ipv4", "192.168.0.0/16", nil)).
		networking()

	result, err := envNet.Subnets(c.Context(), nil)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, subnetInfo("192.168.0.0/16"))
}

func (s *k8sNetworkingSuite) TestCalicoIPPoolV3(c *tc.C) {
	envNet := newClusterBuilder().
		addCRD("ippools", "projectcalico.org", "v3").
		addCR(calicoIPPool("projectcalico.org/v3", "default-ipv4", "192.168.0.0/16", nil)).
		networking()

	result, err := envNet.Subnets(c.Context(), nil)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, subnetInfo("192.168.0.0/16"))
}

func (s *k8sNetworkingSuite) TestCalicoIPPoolDualStack(c *tc.C) {
	envNet := newClusterBuilder().
		addCRD("ippools", "crd.projectcalico.org", "v1").
		addCR(
			calicoIPPool("crd.projectcalico.org/v1", "default-ipv4", "192.168.0.0/16", nil),
			calicoIPPool("crd.projectcalico.org/v1", "default-ipv6", "fd00::/48", nil),
		).
		networking()

	result, err := envNet.Subnets(c.Context(), nil)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, subnetInfo("192.168.0.0/16", "fd00::/48"))
}

func (s *k8sNetworkingSuite) TestCalicoIPPoolExcludesDisabled(c *tc.C) {
	envNet := newClusterBuilder().
		addCRD("ippools", "crd.projectcalico.org", "v1").
		addCR(
			calicoIPPool("crd.projectcalico.org/v1", "enabled", "192.168.0.0/16", nil),
			calicoIPPool("crd.projectcalico.org/v1", "disabled", "10.99.0.0/16", map[string]any{"disabled": true}),
		).
		networking()

	result, err := envNet.Subnets(c.Context(), nil)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, subnetInfo("192.168.0.0/16"))
}

func (s *k8sNetworkingSuite) TestCalicoIPPoolExcludesNonWorkloadAllowedUses(c *tc.C) {
	envNet := newClusterBuilder().
		addCRD("ippools", "crd.projectcalico.org", "v1").
		addCR(
			calicoIPPool("crd.projectcalico.org/v1", "workload", "192.168.0.0/16",
				map[string]any{"allowedUses": []any{"Workload"}}),
			calicoIPPool("crd.projectcalico.org/v1", "tunnel-only", "10.99.0.0/16",
				map[string]any{"allowedUses": []any{"Tunnel"}}),
		).
		networking()

	result, err := envNet.Subnets(c.Context(), nil)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, subnetInfo("192.168.0.0/16"))
}

func (s *k8sNetworkingSuite) TestCalicoIPPoolIncludesAssignmentModeManual(c *tc.C) {
	envNet := newClusterBuilder().
		addCRD("ippools", "crd.projectcalico.org", "v1").
		addCR(calicoIPPool("crd.projectcalico.org/v1", "manual", "192.168.0.0/16",
			map[string]any{"assignmentMode": "Manual"})).
		networking()

	result, err := envNet.Subnets(c.Context(), nil)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, subnetInfo("192.168.0.0/16"))
}

func (s *k8sNetworkingSuite) TestCalicoIPPoolIgnoresDivergentNodeSpec(c *tc.C) {
	// calico-ipam: a populated but divergent node.Spec.PodCIDR must be ignored.
	envNet := newClusterBuilder().
		addCore(node("a", []string{"10.244.0.0/24"}, nil)).
		addCRD("ippools", "crd.projectcalico.org", "v1").
		addCR(calicoIPPool("crd.projectcalico.org/v1", "default-ipv4", "192.168.0.0/16", nil)).
		networking()

	result, err := envNet.Subnets(c.Context(), nil)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, subnetInfo("192.168.0.0/16"))
}

func (s *k8sNetworkingSuite) TestCalicoHostLocalUsesNodeSpec(c *tc.C) {
	// IPPool CRD installed but zero pools => host-local/Canal => node spec.
	envNet := newClusterBuilder().
		addCore(node("a", []string{"10.244.0.0/24"}, nil)).
		addCRD("ippools", "crd.projectcalico.org", "v1").
		networking()

	result, err := envNet.Subnets(c.Context(), nil)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, subnetInfo("10.244.0.0/24"))
}

func (s *k8sNetworkingSuite) TestCalicoHostLocalWithCanalConfig(c *tc.C) {
	envNet := newClusterBuilder().
		addCore(
			node("a", []string{"10.244.1.0/24"}, nil),
			configMap("kube-system", "canal-config", map[string]string{
				"net-conf.json": `{"Network":"10.244.0.0/16","Backend":{"Type":"vxlan"}}`,
			}),
		).
		addCRD("ippools", "crd.projectcalico.org", "v1").
		networking()

	result, err := envNet.Subnets(c.Context(), nil)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, subnetInfo("10.244.0.0/16", "10.244.1.0/24"))
}

func (s *k8sNetworkingSuite) TestCalicoCRDAbsentContributesNothing(c *tc.C) {
	// IPPool CRD absent (distinct from empty pools): a node with the calico
	// host-IP annotation must NOT be consumed; result falls through to the
	// fallback (no node spec, no other source).
	envNet := newClusterBuilder().
		addCore(node("a", nil, map[string]string{"projectcalico.org/IPv4Address": "10.0.0.5/24"})).
		networking()

	result, err := envNet.Subnets(c.Context(), nil)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, fallbackResult)
}

func (s *k8sNetworkingSuite) TestCalicoListForbiddenWithPopulatedNodeYieldsFallback(c *tc.C) {
	b := newClusterBuilder().
		addCore(node("a", []string{"10.244.0.0/24"}, nil)).
		addCRD("ippools", "crd.projectcalico.org", "v1")
	typed, apiext, dyn := b.build()
	dyn.PrependReactor("list", "ippools", func(k8stesting.Action) (bool, k8sruntime.Object, error) {
		return true, nil, forbidden("ippools")
	})
	en := newEnvironNetworking(typed, apiext, dyn)

	result, err := en.Subnets(c.Context(), nil)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, fallbackResult)
}

// -----------------------------------------------------------------------------
// Cilium
// -----------------------------------------------------------------------------

func ciliumConfig(data map[string]string) *corev1.ConfigMap {
	return configMap("kube-system", "cilium-config", data)
}

func (s *k8sNetworkingSuite) TestCiliumClusterPoolConfigMap(c *tc.C) {
	envNet := newClusterBuilder().
		addCore(
			node("a", []string{"10.244.0.0/24"}, nil), // divergent node spec, must be ignored
			ciliumConfig(map[string]string{
				"ipam":                   "cluster-pool",
				"cluster-pool-ipv4-cidr": "10.0.0.0/8",
				"cluster-pool-ipv6-cidr": "fd00::/48",
			}),
		).
		networking()

	result, err := envNet.Subnets(c.Context(), nil)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, subnetInfo("10.0.0.0/8", "fd00::/48"))
}

func (s *k8sNetworkingSuite) TestCiliumClusterPoolDefaultModeFromConfigMap(c *tc.C) {
	// ipam key absent => default cluster-pool.
	envNet := newClusterBuilder().
		addCore(ciliumConfig(map[string]string{
			"cluster-pool-ipv4-cidr": "10.0.0.0/8",
		})).
		networking()

	result, err := envNet.Subnets(c.Context(), nil)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, subnetInfo("10.0.0.0/8"))
}

func (s *k8sNetworkingSuite) TestCiliumClusterPoolFromCiliumNode(c *tc.C) {
	envNet := newClusterBuilder().
		addCore(ciliumConfig(map[string]string{"ipam": "cluster-pool"})).
		addCRD("ciliumnodes", "cilium.io", "v2").
		addCR(cr("cilium.io/v2", "CiliumNode", "node-a", map[string]any{
			"ipam": map[string]any{"podCIDRs": []any{"10.1.0.0/24"}},
		})).
		networking()

	result, err := envNet.Subnets(c.Context(), nil)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, subnetInfo("10.1.0.0/24"))
}

func (s *k8sNetworkingSuite) TestCiliumMultiPoolFromCRD(c *tc.C) {
	envNet := newClusterBuilder().
		addCore(ciliumConfig(map[string]string{"ipam": "multi-pool"})).
		addCRD("ciliumpodippools", "cilium.io", "v2alpha1").
		addCR(cr("cilium.io/v2alpha1", "CiliumPodIPPool", "default", map[string]any{
			"ipv4": map[string]any{"cidrs": []any{"10.10.0.0/16"}},
			"ipv6": map[string]any{"cidrs": []any{"fd00::/48"}},
		})).
		networking()

	result, err := envNet.Subnets(c.Context(), nil)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, subnetInfo("10.10.0.0/16", "fd00::/48"))
}

func (s *k8sNetworkingSuite) TestCiliumKubernetesModeUsesNodeSpec(c *tc.C) {
	envNet := newClusterBuilder().
		addCore(
			node("a", []string{"10.244.0.0/24"}, nil),
			ciliumConfig(map[string]string{"ipam": "kubernetes"}),
		).
		networking()

	result, err := envNet.Subnets(c.Context(), nil)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, subnetInfo("10.244.0.0/24"))
}

func (s *k8sNetworkingSuite) TestCiliumVPCNativeModesYieldFallback(c *tc.C) {
	for _, mode := range []string{"eni", "azure", "alibabacloud", "crd"} {
		envNet := newClusterBuilder().
			addCore(
				node("a", []string{"10.244.0.0/24"}, nil),
				ciliumConfig(map[string]string{"ipam": mode}),
			).
			networking()

		result, err := envNet.Subnets(c.Context(), nil)

		c.Assert(err, tc.ErrorIsNil, tc.Commentf("mode %q", mode))
		c.Check(result, tc.DeepEquals, fallbackResult, tc.Commentf("mode %q", mode))
	}
}

func (s *k8sNetworkingSuite) TestCiliumAnnotationNotConsumed(c *tc.C) {
	// No cilium-config, no CRDs, no node spec: the cilium.io/*-pod-cidr
	// annotation must not be consumed; result is the fallback.
	envNet := newClusterBuilder().
		addCore(node("a", nil, map[string]string{"cilium.io/ipv4-pod-cidr": "10.5.0.0/16"})).
		networking()

	result, err := envNet.Subnets(c.Context(), nil)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, fallbackResult)
}

// -----------------------------------------------------------------------------
// OVN-Kubernetes
// -----------------------------------------------------------------------------

func (s *k8sNetworkingSuite) TestOVNNodeSubnetsSingleStackString(c *tc.C) {
	envNet := newClusterBuilder().
		addCore(node("a", nil, map[string]string{
			"k8s.ovn.org/node-subnets": `{"default":"10.130.0.0/23"}`,
		})).
		networking()

	result, err := envNet.Subnets(c.Context(), nil)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, subnetInfo("10.130.0.0/23"))
}

func (s *k8sNetworkingSuite) TestOVNNodeSubnetsDualStackArray(c *tc.C) {
	envNet := newClusterBuilder().
		addCore(node("a", nil, map[string]string{
			"k8s.ovn.org/node-subnets": `{"default":["10.130.0.0/23","fd01:0:0:2::/64"]}`,
		})).
		networking()

	result, err := envNet.Subnets(c.Context(), nil)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, subnetInfo("10.130.0.0/23", "fd01:0:0:2::/64"))
}

func (s *k8sNetworkingSuite) TestOVNNodeSubnetsExcludesNonDefault(c *tc.C) {
	envNet := newClusterBuilder().
		addCore(node("a", nil, map[string]string{
			"k8s.ovn.org/node-subnets":               `{"default":"10.130.0.0/23","secondary":"10.140.0.0/23"}`,
			"k8s.ovn.org/hybrid-overlay-node-subnet": "10.150.0.0/23",
		})).
		networking()

	result, err := envNet.Subnets(c.Context(), nil)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, subnetInfo("10.130.0.0/23"))
}

func (s *k8sNetworkingSuite) TestOVNConfigNetCIDRHostPrefixStripped(c *tc.C) {
	envNet := newClusterBuilder().
		addCore(configMap("ovn-kubernetes", "ovn-config", map[string]string{
			"net_cidr": "10.128.0.0/14/23,fd01::/48/64",
		})).
		networking()

	result, err := envNet.Subnets(c.Context(), nil)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, subnetInfo("10.128.0.0/14", "fd01::/48"))
}

func (s *k8sNetworkingSuite) TestOVNConfigNetCIDROpenShiftNamespace(c *tc.C) {
	envNet := newClusterBuilder().
		addCore(configMap("openshift-ovn-kubernetes", "ovn-config", map[string]string{
			"net_cidr": "10.128.0.0/14/23",
		})).
		networking()

	result, err := envNet.Subnets(c.Context(), nil)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, subnetInfo("10.128.0.0/14"))
}

func (s *k8sNetworkingSuite) TestOVNOpenShiftNetworkCRD(c *tc.C) {
	netObj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "config.openshift.io/v1",
		"kind":       "Network",
		"metadata":   map[string]any{"name": "cluster"},
		"status": map[string]any{
			"clusterNetwork": []any{
				map[string]any{"cidr": "10.128.0.0/14", "hostPrefix": int64(23)},
			},
		},
	}}
	envNet := newClusterBuilder().
		addCRD("networks", "config.openshift.io", "v1").
		addCR(netObj).
		networking()

	result, err := envNet.Subnets(c.Context(), nil)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, subnetInfo("10.128.0.0/14"))
}

func (s *k8sNetworkingSuite) TestOVNUnionOfSources(c *tc.C) {
	netObj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "config.openshift.io/v1",
		"kind":       "Network",
		"metadata":   map[string]any{"name": "cluster"},
		"status": map[string]any{
			"clusterNetwork": []any{map[string]any{"cidr": "10.128.0.0/14"}},
		},
	}}
	envNet := newClusterBuilder().
		addCore(
			node("a", nil, map[string]string{"k8s.ovn.org/node-subnets": `{"default":"10.130.0.0/23"}`}),
			configMap("ovn-kubernetes", "ovn-config", map[string]string{"net_cidr": "10.128.0.0/14/23"}),
		).
		addCRD("networks", "config.openshift.io", "v1").
		addCR(netObj).
		networking()

	result, err := envNet.Subnets(c.Context(), nil)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, subnetInfo("10.128.0.0/14", "10.130.0.0/23"))
}

// -----------------------------------------------------------------------------
// Kube-OVN
// -----------------------------------------------------------------------------

func kubeOVNSubnet(name string, spec map[string]any) *unstructured.Unstructured {
	return cr("kubeovn.io/v1", "Subnet", name, spec)
}

func (s *k8sNetworkingSuite) TestKubeOVNDefaultSubnet(c *tc.C) {
	envNet := newClusterBuilder().
		addCRD("subnets", "kubeovn.io", "v1").
		addCR(
			kubeOVNSubnet("ovn-default", map[string]any{"cidrBlock": "10.16.0.0/16", "default": true}),
			kubeOVNSubnet("join", map[string]any{"cidrBlock": "100.64.0.0/16"}),
		).
		networking()

	result, err := envNet.Subnets(c.Context(), nil)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, subnetInfo("10.16.0.0/16"))
}

func (s *k8sNetworkingSuite) TestKubeOVNExcludesJoinByCIDRAnnotation(c *tc.C) {
	// Join subnet uses a custom name but is identified via the node cidr
	// annotation. The node ovn.kubernetes.io/cidr value must not be consumed
	// as a pod CIDR.
	envNet := newClusterBuilder().
		addCore(node("a", nil, map[string]string{
			"ovn.kubernetes.io/logical_switch": "transit",
			"ovn.kubernetes.io/cidr":           "100.64.0.0/16",
		})).
		addCRD("subnets", "kubeovn.io", "v1").
		addCR(
			kubeOVNSubnet("ovn-default", map[string]any{"cidrBlock": "10.16.0.0/16", "default": true}),
			kubeOVNSubnet("transit", map[string]any{"cidrBlock": "100.64.0.0/16"}),
		).
		networking()

	result, err := envNet.Subnets(c.Context(), nil)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, subnetInfo("10.16.0.0/16"))
}

func (s *k8sNetworkingSuite) TestKubeOVNNodeListErrorFallsBack(c *tc.C) {
	typed, apiext, dyn := newClusterBuilder().
		addCRD("subnets", "kubeovn.io", "v1").
		addCR(
			kubeOVNSubnet("ovn-default", map[string]any{"cidrBlock": "10.16.0.0/16", "default": true}),
			kubeOVNSubnet("transit", map[string]any{"cidrBlock": "100.64.0.0/16"}),
		).
		build()
	typed.PrependReactor("list", "nodes", func(k8stesting.Action) (bool, k8sruntime.Object, error) {
		return true, nil, errors.New("boom")
	})
	envNet := newEnvironNetworking(typed, apiext, dyn)

	result, err := envNet.Subnets(c.Context(), nil)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, fallbackResult)
}

func (s *k8sNetworkingSuite) TestKubeOVNIncludesDisableInterConnectionPodSubnet(c *tc.C) {
	envNet := newClusterBuilder().
		addCRD("subnets", "kubeovn.io", "v1").
		addCR(
			kubeOVNSubnet("ovn-default", map[string]any{"cidrBlock": "10.16.0.0/16", "default": true}),
			kubeOVNSubnet("custom", map[string]any{"cidrBlock": "10.17.0.0/16", "disableInterConnection": true}),
			kubeOVNSubnet("join", map[string]any{"cidrBlock": "100.64.0.0/16", "disableInterConnection": true}),
		).
		networking()

	result, err := envNet.Subnets(c.Context(), nil)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, subnetInfo("10.16.0.0/16", "10.17.0.0/16"))
}

func (s *k8sNetworkingSuite) TestKubeOVNIncludesPerNamespaceSubnet(c *tc.C) {
	envNet := newClusterBuilder().
		addCRD("subnets", "kubeovn.io", "v1").
		addCR(
			kubeOVNSubnet("ovn-default", map[string]any{"cidrBlock": "10.16.0.0/16", "default": true}),
			kubeOVNSubnet("ns-subnet", map[string]any{
				"cidrBlock":  "10.18.0.0/16",
				"namespaces": []any{"team-a"},
			}),
			kubeOVNSubnet("join", map[string]any{"cidrBlock": "100.64.0.0/16"}),
		).
		networking()

	result, err := envNet.Subnets(c.Context(), nil)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, subnetInfo("10.16.0.0/16", "10.18.0.0/16"))
}

func (s *k8sNetworkingSuite) TestKubeOVNDualStackSplit(c *tc.C) {
	envNet := newClusterBuilder().
		addCRD("subnets", "kubeovn.io", "v1").
		addCR(kubeOVNSubnet("ovn-default", map[string]any{
			"cidrBlock": "10.16.0.0/16,fd00:10:16::/112",
			"protocol":  "Dual",
			"default":   true,
		})).
		networking()

	result, err := envNet.Subnets(c.Context(), nil)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, subnetInfo("10.16.0.0/16", "fd00:10:16::/112"))
}

func (s *k8sNetworkingSuite) TestKubeOVNDefaultVPCPreferred(c *tc.C) {
	envNet := newClusterBuilder().
		addCRD("subnets", "kubeovn.io", "v1").
		addCR(
			kubeOVNSubnet("ovn-default", map[string]any{"cidrBlock": "10.16.0.0/16", "vpc": "ovn-cluster"}),
			kubeOVNSubnet("custom-vpc", map[string]any{"cidrBlock": "10.99.0.0/16", "vpc": "tenant"}),
		).
		networking()

	result, err := envNet.Subnets(c.Context(), nil)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, subnetInfo("10.16.0.0/16"))
}

// -----------------------------------------------------------------------------
// Resolver / chain semantics
// -----------------------------------------------------------------------------

func (s *k8sNetworkingSuite) TestChainFirstMatchShortCircuits(c *tc.C) {
	// Calico matches; Kube-OVN Subnets also exist but must not be consulted.
	envNet := newClusterBuilder().
		addCRD("ippools", "crd.projectcalico.org", "v1").
		addCRD("subnets", "kubeovn.io", "v1").
		addCR(
			calicoIPPool("crd.projectcalico.org/v1", "default-ipv4", "192.168.0.0/16", nil),
			kubeOVNSubnet("ovn-default", map[string]any{"cidrBlock": "10.16.0.0/16", "default": true}),
		).
		networking()

	result, err := envNet.Subnets(c.Context(), nil)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, subnetInfo("192.168.0.0/16"))
}

func (s *k8sNetworkingSuite) TestChainAllEmptyFallsBack(c *tc.C) {
	envNet := newClusterBuilder().networking()

	result, err := envNet.Subnets(c.Context(), nil)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, fallbackResult)
}

func (s *k8sNetworkingSuite) TestResolveSkipsErroringDiscoverer(c *tc.C) {
	clients := Clients{}
	cidrs := Resolve(c.Context(), clients,
		stubDiscoverer{name: "boom", err: errors.New("boom")},
		stubDiscoverer{name: "match", cidrs: []string{"10.0.0.0/24"}},
	)
	c.Check(cidrs, tc.DeepEquals, []string{"10.0.0.0/24"})
}

func (s *k8sNetworkingSuite) TestResolveStopsAtFirstNonEmpty(c *tc.C) {
	clients := Clients{}
	cidrs := Resolve(c.Context(), clients,
		stubDiscoverer{name: "first", cidrs: []string{"10.0.0.0/24"}},
		stubDiscoverer{name: "second", cidrs: []string{"10.1.0.0/24"}},
	)
	c.Check(cidrs, tc.DeepEquals, []string{"10.0.0.0/24"})
}

type stubDiscoverer struct {
	name  string
	cidrs []string
	err   error
}

func (d stubDiscoverer) Name() string { return d.name }
func (d stubDiscoverer) Discover(context.Context, Clients) ([]string, error) {
	return d.cidrs, d.err
}

// -----------------------------------------------------------------------------
// Cloud-aware fallback warning
// -----------------------------------------------------------------------------

func (s *k8sNetworkingSuite) TestManagedCloudDetected(c *tc.C) {
	clients := newClusterBuilder().addCore(
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{
			Name:   "a",
			Labels: map[string]string{"eks.amazonaws.com/nodegroup": "ng-1"},
		}},
	).networking().clients()

	c.Check(detectManagedCloud(c.Context(), clients), tc.Equals, "ec2")
}

func (s *k8sNetworkingSuite) TestManagedCloudExcludesMicroK8s(c *tc.C) {
	clients := newClusterBuilder().addCore(
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{
			Name:   "a",
			Labels: map[string]string{"microk8s.io/cluster": "true"},
		}},
	).networking().clients()

	c.Check(detectManagedCloud(c.Context(), clients), tc.Equals, "")
}

// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package subnet

import (
	"errors"
	"maps"

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

	"github.com/juju/juju/core/network"
)

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

func (b *clusterBuilder) clients() Clients {
	typed, apiext, dyn := b.build()
	return Clients{
		Typed:         typed,
		Dynamic:       dyn,
		APIExtensions: apiext,
		CloudDetector: testCloudDetector,
	}
}

func testCloudDetector(node corev1.Node) (string, string) {
	labels := node.GetLabels()
	if _, ok := labels["eks.amazonaws.com/nodegroup"]; ok {
		return "ec2", ""
	}
	if _, ok := labels["kubernetes.azure.com/cluster"]; ok {
		return "azure", ""
	}
	if _, ok := labels["cloud.google.com/gke-nodepool"]; ok {
		return "gce", ""
	}
	if _, ok := labels["microk8s.io/cluster"]; ok {
		return "microk8s", ""
	}
	return "", ""
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

func calicoIPPool(apiVersion, name, cidr string, extra map[string]any) *unstructured.Unstructured {
	spec := map[string]any{"cidr": cidr}
	maps.Copy(spec, extra)
	return cr(apiVersion, "IPPool", name, spec)
}

func ciliumConfig(data map[string]string) *corev1.ConfigMap {
	return configMap("kube-system", "cilium-config", data)
}

func kubeOVNSubnet(name string, spec map[string]any) *unstructured.Unstructured {
	return cr("kubeovn.io/v1", "Subnet", name, spec)
}

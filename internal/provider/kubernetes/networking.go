// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	"context"
	"net"
	"sort"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	k8s "github.com/juju/juju/caas/kubernetes"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
)

var _ environs.Networking = (*environNetworking)(nil)

// fallbackCIDRs is the catch-all pod CIDR set (all IPv4 + IPv6). It is returned
// when no CNI source can be confidently identified, and also directly by a
// discoverer that hits an RBAC denial (a present-but-unreadable CNI source) so
// that the resolver short-circuits to the safe fallback rather than degrading
// to a possibly-divergent node range.
var fallbackCIDRs = []string{"0.0.0.0/0", "::/0"}

// Names used purely for logging/tracing of each discoverer.
const (
	calicoDiscovererName  = "calico"
	ciliumDiscovererName  = "cilium"
	ovnDiscovererName     = "ovn-kubernetes"
	kubeOVNDiscovererName = "kube-ovn"
	nodeDiscovererName    = "node-pod-cidr"
)

type environNetworking struct {
	environs.NoContainerAddressesEnviron
	environs.NoSpaceDiscoveryEnviron

	clientset     kubernetes.Interface
	dynamicClient dynamic.Interface
	apiExtClient  apiextensionsclientset.Interface
}

func newEnvironNetworking(
	k8sClient kubernetes.Interface,
	apiExtClient apiextensionsclientset.Interface,
	dynamicClient dynamic.Interface,
) environNetworking {
	return environNetworking{
		clientset:     k8sClient,
		dynamicClient: dynamicClient,
		apiExtClient:  apiExtClient,
	}
}

// Clients bundles the Kubernetes clients each discoverer may use: the typed
// clientset (core resources: Nodes, ConfigMaps), the dynamic client (CNI custom
// resource objects, read as unstructured with an explicit GVR), and the
// apiextensions client (CRD definitions — installation and served-version
// detection).
type Clients struct {
	Typed         kubernetes.Interface
	Dynamic       dynamic.Interface
	APIExtensions apiextensionsclientset.Interface
}

func (en environNetworking) clients() Clients {
	return Clients{
		Typed:         en.clientset,
		Dynamic:       en.dynamicClient,
		APIExtensions: en.apiExtClient,
	}
}

// Discoverer recognises and reads the pod CIDRs for one IP-allocation strategy.
type Discoverer interface {
	// Name identifies the strategy (for logging/tracing).
	Name() string
	// Discover returns the pod CIDRs when this discoverer positively identifies
	// its source, or an empty slice when its source is absent / not applicable
	// (so the resolver moves on). An error is returned only for genuinely
	// unexpected failures; the resolver logs it and continues. Discovery never
	// fails the caller.
	Discover(ctx context.Context, clients Clients) ([]string, error)
}

// Resolve walks the chain and returns the CIDRs from the first discoverer that
// yields a confident, non-empty result; if every discoverer is empty it returns
// nil. A discoverer that hits an RBAC denial returns the fallback CIDRs, which
// (being non-empty) short-circuits the chain here.
func Resolve(ctx context.Context, clients Clients, chain ...Discoverer) []string {
	for _, d := range chain {
		cidrs, err := d.Discover(ctx, clients)
		if err != nil {
			logger.Debugf(ctx, "pod-subnet discoverer %q failed, skipping: %v", d.Name(), err)
			continue
		}
		if len(cidrs) > 0 {
			logger.Debugf(ctx, "pod-subnet discoverer %q matched CIDRs %v", d.Name(), cidrs)
			return cidrs
		}
	}
	return nil
}

// Subnets is part of the [environs.Networking] interface.
func (en environNetworking) Subnets(ctx context.Context, _ []network.Id) ([]network.SubnetInfo, error) {
	clients := en.clients()
	if clients.Typed == nil {
		return network.FallbackSubnetInfo, nil
	}

	chain := []Discoverer{
		calicoDiscoverer{},
		ciliumDiscoverer{},
		ovnKubernetesDiscoverer{},
		kubeOVNDiscoverer{},
		nodePodCIDRDiscoverer{},
	}

	cidrs := Resolve(ctx, clients, chain...)
	subnets := normalizeSubnets(ctx, cidrs)
	if len(subnets) == 0 {
		logFallbackWarning(ctx, clients)
		return network.FallbackSubnetInfo, nil
	}
	if isFallbackSubnets(subnets) {
		// A discoverer short-circuited to the fallback (RBAC denial). It has
		// already logged a remediation warning; return the canonical fallback
		// with an empty ProviderId to keep persistence idempotent.
		return network.FallbackSubnetInfo, nil
	}
	return subnets, nil
}

// normalizeSubnets parses each candidate CIDR, replaces it with its masked
// canonical form, dedupes (on the masked form) and sorts. Invalid CIDRs are
// logged at debug and skipped. Child blocks are not collapsed into supernets.
func normalizeSubnets(ctx context.Context, cidrs []string) []network.SubnetInfo {
	seen := make(map[string]struct{})
	result := make([]network.SubnetInfo, 0, len(cidrs))
	for _, candidate := range cidrs {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		_, ipNet, err := net.ParseCIDR(candidate)
		if err != nil {
			logger.Debugf(ctx, "ignoring invalid pod CIDR %q: %v", candidate, err)
			continue
		}
		cidr := ipNet.String()
		if _, exists := seen[cidr]; exists {
			continue
		}
		seen[cidr] = struct{}{}
		result = append(result, network.SubnetInfo{
			CIDR:       cidr,
			ProviderId: network.Id(cidr),
		})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].CIDR < result[j].CIDR
	})
	return result
}

// isFallbackSubnets reports whether the normalised result is exactly the
// catch-all fallback set (and therefore should be returned as the canonical
// [network.FallbackSubnetInfo]).
func isFallbackSubnets(subnets []network.SubnetInfo) bool {
	if len(subnets) != len(network.FallbackSubnetInfo) {
		return false
	}
	want := set.NewStrings()
	for _, s := range network.FallbackSubnetInfo {
		want.Add(s.CIDR)
	}
	for _, s := range subnets {
		if !want.Contains(s.CIDR) {
			return false
		}
	}
	return true
}

// logFallbackWarning emits a warning when discovery found nothing. On a managed
// VPC-native cluster (EKS/AKS/GKE, excluding MicroK8s) it explains why there is
// no pod CIDR to discover; otherwise it logs a generic notice.
func logFallbackWarning(ctx context.Context, clients Clients) {
	switch detectManagedCloud(ctx, clients) {
	case k8s.K8sCloudEC2, k8s.K8sCloudAzure, k8s.K8sCloudGCE:
		logger.Warningf(ctx,
			"no pod subnets discovered: this looks like a managed cloud cluster "+
				"(EKS/AKS/GKE) using VPC-native networking, where pods receive "+
				"routable cloud IPs and there is no pod CIDR to discover. Install a "+
				"supported overlay CNI (Calico, Cilium, kube-router, OVN-Kubernetes "+
				"or Kube-OVN) if subnet discovery is required. Using the default %v "+
				"fallback subnets.", fallbackCIDRs)
	default:
		logger.Warningf(ctx,
			"no pod subnets discovered; using the default %v fallback subnets.",
			fallbackCIDRs)
	}
}

// detectManagedCloud returns the managed cloud type (EC2/Azure/GCE) when any
// node matches its checkers, or "" otherwise. MicroK8s is excluded as it ships
// its own CNI.
func detectManagedCloud(ctx context.Context, clients Clients) string {
	nodes, err := clients.Typed.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		logger.Debugf(ctx, "unable to list nodes for cloud detection: %v", err)
		return ""
	}
	for _, node := range nodes.Items {
		cloudType, _ := getCloudRegionFromNodeMeta(node)
		switch cloudType {
		case k8s.K8sCloudEC2, k8s.K8sCloudAzure, k8s.K8sCloudGCE:
			return cloudType
		}
	}
	return ""
}

// deniedReadingResource classifies an error from a pod-CIDR read. It reports
// whether the error was an RBAC denial (Forbidden): when true, the caller must
// return the fallback CIDRs to short-circuit the chain, and a warning with
// remediation guidance has been logged. Absent / no-match / transient errors
// are logged at debug and false is returned, so the caller contributes nothing.
func deniedReadingResource(ctx context.Context, discoverer, resource string, err error) bool {
	if k8serrors.IsForbidden(err) {
		logger.Warningf(ctx,
			"%s pod-subnet discovery: permission denied reading %s; grant the "+
				"model's Kubernetes credential read access to %s to enable accurate "+
				"pod-subnet discovery. Using the default %v fallback subnets: %v",
			discoverer, resource, resource, fallbackCIDRs, err)
		return true
	}
	switch {
	case k8serrors.IsNotFound(err),
		meta.IsNoMatchError(err),
		discovery.IsGroupDiscoveryFailedError(err):
		logger.Debugf(ctx, "%s pod-subnet discovery: %s not present; skipping: %v",
			discoverer, resource, err)
	default:
		logger.Debugf(ctx, "%s pod-subnet discovery: ignoring error reading %s: %v",
			discoverer, resource, err)
	}
	return false
}

// firstServedCRD resolves the first of the named CRDs (each "<plural>.<group>")
// that is installed and has a served version, returning a GVR built from that
// served version. found is false when none are installed/served. denied is true
// when reading the CRD definitions was Forbidden, in which case the caller must
// short-circuit to the fallback.
func firstServedCRD(
	ctx context.Context, clients Clients, discoverer string, names ...string,
) (gvr schema.GroupVersionResource, found bool, denied bool) {
	if clients.APIExtensions == nil {
		return schema.GroupVersionResource{}, false, false
	}
	for _, name := range names {
		crd, err := clients.APIExtensions.ApiextensionsV1().
			CustomResourceDefinitions().Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			if deniedReadingResource(ctx, discoverer, "the "+name+" CRD definition", err) {
				return schema.GroupVersionResource{}, false, true
			}
			continue
		}
		for _, v := range crd.Spec.Versions {
			if !v.Served {
				continue
			}
			return schema.GroupVersionResource{
				Group:    crd.Spec.Group,
				Version:  v.Name,
				Resource: crd.Spec.Names.Plural,
			}, true, false
		}
		logger.Debugf(ctx, "%s pod-subnet discovery: CRD %q installed but has no served version",
			discoverer, name)
	}
	return schema.GroupVersionResource{}, false, false
}

// nodeSpecCIDRs returns the union of node.Spec.PodCIDR(s) across all nodes.
// denied reports an RBAC denial reading Nodes.
func nodeSpecCIDRs(ctx context.Context, clients Clients, discoverer string) (cidrs []string, denied bool) {
	nodes, err := clients.Typed.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, deniedReadingResource(ctx, discoverer, "Nodes", err)
	}
	for _, node := range nodes.Items {
		cidrs = append(cidrs, node.Spec.PodCIDRs...)
		if node.Spec.PodCIDR != "" {
			cidrs = append(cidrs, node.Spec.PodCIDR)
		}
	}
	return cidrs, false
}

// kubeRouterPodCIDRAnnotationKeys are the kube-router pod-CIDR override
// annotations: the current plural key (v2.0+, comma-separated, dual-stack) and
// the legacy singular key. kube-router never writes them itself, but an admin
// may. Both are read and unioned with node.Spec.PodCIDR(s).
var kubeRouterPodCIDRAnnotationKeys = []string{
	"kube-router.io/pod-cidrs",
	"kube-router.io/pod-cidr",
}

// nodePodCIDRDiscoverer is the final, generic catch-all link. A populated
// node.Spec.PodCIDR(s) is authoritative only for CNIs that allocate pod IPs
// from kube-controller-manager node-CIDR allocation; the CNI-specific
// discoverers run first and read the node spec themselves for their
// kcm-allocated modes, so by the time control reaches here the remaining real
// consumer is kube-router (which has no CRD/ConfigMap to key on).
type nodePodCIDRDiscoverer struct{}

func (nodePodCIDRDiscoverer) Name() string { return nodeDiscovererName }

func (nodePodCIDRDiscoverer) Discover(ctx context.Context, clients Clients) ([]string, error) {
	if clients.Typed == nil {
		return nil, nil
	}
	nodes, err := clients.Typed.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		if deniedReadingResource(ctx, nodeDiscovererName, "Nodes", err) {
			return fallbackCIDRs, nil
		}
		return nil, nil
	}
	var out []string
	for _, node := range nodes.Items {
		out = append(out, node.Spec.PodCIDRs...)
		if node.Spec.PodCIDR != "" {
			out = append(out, node.Spec.PodCIDR)
		}
		for _, key := range kubeRouterPodCIDRAnnotationKeys {
			raw := strings.TrimSpace(node.Annotations[key])
			if raw == "" {
				continue
			}
			out = append(out, splitCIDRCandidates(raw)...)
		}
	}
	return out, nil
}

// splitCIDRCandidates splits a raw annotation/ConfigMap value into individual
// CIDR candidates on commas, semicolons and whitespace.
func splitCIDRCandidates(raw string) []string {
	return strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\t' || r == '\n'
	})
}

// NetworkInterfaces is part of the [environs.Networking] interface.
func (environNetworking) NetworkInterfaces(ctx context.Context, ids []instance.Id) ([]network.InterfaceInfos, error) {
	return nil, errors.NotSupportedf("network interfaces")
}

// SupportsSpaces is part of the [environs.Networking] interface.
func (environNetworking) SupportsSpaces() (bool, error) {
	return false, nil
}

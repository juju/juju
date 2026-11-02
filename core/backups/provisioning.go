// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

// KubernetesProvisioning records the running controller's substrate. Images
// are source pod references and may be tagged or digest-addressed.
type KubernetesProvisioning struct {
	AgentImage         string
	CharmImage         string
	Architecture       string
	StorageClass       string
	StorageSizeMiB     uint64
	ServiceType        string
	ExternalName       string
	ExternalIPs        []string
	LoadBalancerIP     string
	ServiceAnnotations map[string]string
}

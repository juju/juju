// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

// IsLegacyLabels indicates if this provider is operating on a legacy label schema
func (k *kubernetesClient) IsLegacyLabels() bool {
	return k.isLegacyLabels
}

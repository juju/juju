// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

func (k *kubernetesClient) IsLegacyLabels() bool {
	return k.isLegacyLabels
}

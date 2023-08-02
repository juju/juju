// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	"github.com/juju/charm/v11"
	"github.com/juju/collections/set"

	corebase "github.com/juju/juju/core/base"
)

// IsKubernetes reports whether the given charm should be deployed to
// Kubernetes, that is, a v1 charm with series "kubernetes", or a v2 charm
// with containers specified.
func IsKubernetes(cm charm.CharmMeta) bool {
	switch charm.MetaFormat(cm) {
	case charm.FormatV2:
		if len(cm.Meta().Containers) > 0 {
			return true
		}
		// TODO (hml) 2021-04-19
		// Enhance with logic around Assumes once that is finalized.
	case charm.FormatV1:
		if set.NewStrings(cm.Meta().Series...).Contains(corebase.Kubernetes.String()) {
			return true
		}
	}
	return false
}

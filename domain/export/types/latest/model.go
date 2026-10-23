// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package latest

import (
	v4_2_0 "github.com/juju/juju/domain/export/types/v4_2_0"
)

// ModelExport is the current target model-export payload type. It tracks the
// last entry of [github.com/juju/juju/domain/export.ExportVersions] / the
// version returned by export.LatestSupportedPayloadVersion.
type ModelExport = v4_2_0.ModelExport

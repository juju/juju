// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The all package facilitates the registration of Juju components into
// the relevant machinery. It is intended as the one place in Juju where
// the components (horizontal design layers) and the machinery
// (vertical/architectural layers) intersect. This approach helps
// alleviate interdependence between the components and the machinery.
//
// This is done in an independent package to avoid circular imports.
package all

import (
	_ "github.com/juju/juju/process/all"
)

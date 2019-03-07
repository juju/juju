// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancemutater

import "github.com/juju/juju/api/instancemutater"

func UnitsChanged(logger Logger, m instancemutater.MutaterMachine, names []string) error {
	return unitsChanged(logger, m, names)
}

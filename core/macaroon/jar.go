// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package macaroon

import (
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/checkers"
)

// MacaroonURI is use when register new Juju checkers with the bakery.
const MacaroonURI = "github.com/juju/juju"

// MacaroonNamespace is the namespace Juju uses for managing macaroons.
var MacaroonNamespace = checkers.NewNamespace(map[string]string{MacaroonURI: ""})

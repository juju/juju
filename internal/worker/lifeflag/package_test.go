// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lifeflag_test

import (
	"github.com/juju/names/v6"

	"github.com/juju/juju/core/life"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package lifeflag_test -destination facade_mocks_test.go github.com/juju/juju/internal/worker/lifeflag Facade

var tag = names.NewUnitTag("blah/123")

func explode(life.Value) bool { panic("unexpected") }
func never(life.Value) bool   { return false }

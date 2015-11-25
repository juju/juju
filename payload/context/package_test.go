// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	"testing"

	gc "gopkg.in/check.v1"

	components "github.com/juju/juju/component/all"
)

func Test(t *testing.T) {
	err := components.RegisterForServer()
	if err != nil {
		t.Fatalf("could not register server components: %v", err)
	}

	gc.TestingT(t)
}

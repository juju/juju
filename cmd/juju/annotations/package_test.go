// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package annotations

import (
	stdtesting "testing"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/jujuclient"
)

//go:generate go run go.uber.org/mock/mockgen -package annotations_test -destination mocks_test.go github.com/juju/juju/cmd/juju/annotations GetAnnotationsAPI,SetAnnotationsAPI

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}

// NewSetCommandForTest returns an annotations command for testing.
func NewSetCommandForTest(store jujuclient.ClientStore, api SetAnnotationsAPI) *setAnnotationsCommand {
	c := &setAnnotationsCommand{
		annotationsAPIFunc: func() (SetAnnotationsAPI, error) { return api, nil },
	}
	c.SetClientStore(store)
	return c
}

// NewGetCommandForTest returns an annotations command for testing.
func NewGetCommandForTest(store jujuclient.ClientStore, api GetAnnotationsAPI) *getAnnotationsCommand {
	c := &getAnnotationsCommand{
		annotationsAPIFunc: func() (GetAnnotationsAPI, error) { return api, nil },
	}
	c.SetClientStore(store)
	return c
}

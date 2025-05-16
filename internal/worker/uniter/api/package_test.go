// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	stdtesting "testing"

	"github.com/juju/tc"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package api -destination uniter_mocks.go -source=./interface_generics.go
//go:generate go run go.uber.org/mock/mockgen -typed -package api -destination domain_mocks.go github.com/juju/juju/internal/worker/uniter/api Unit,Relation,RelationUnit,Application,Charm
//go:generate go run go.uber.org/mock/mockgen -typed -package api -destination secrets_mocks.go github.com/juju/juju/internal/worker/uniter/api SecretsClient,SecretsBackend

func TestPackage(t *stdtesting.T) {
	tc.TestingT(t)
}

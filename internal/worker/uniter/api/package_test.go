// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

//go:generate go run github.com/canonical/gomock/mockgen -package api -destination uniter_mocks_test.go github.com/juju/juju/internal/worker/uniter/api UniterClient
//go:generate go run github.com/canonical/gomock/mockgen -package api -destination domain_mocks_test.go github.com/juju/juju/internal/worker/uniter/api Unit,Relation,RelationUnit,Application,Charm
//go:generate go run github.com/canonical/gomock/mockgen -package api -destination secrets_mocks_test.go github.com/juju/juju/internal/worker/uniter/api SecretsClient,SecretsBackend

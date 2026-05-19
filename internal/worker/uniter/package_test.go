// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

//go:generate go run github.com/canonical/gomock/mockgen -package uniter_test -destination entity_mocks_gen_test.go github.com/juju/juju/internal/worker/uniter/api Application,Unit,Relation,RelationUnit,UniterClient,SecretsClient,SecretsBackend,Charm

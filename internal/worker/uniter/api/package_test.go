// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

//go:generate go run github.com/canonical/gomock/mockgen -package mocks -destination mocks/uniter_mocks.go github.com/juju/juju/internal/worker/uniter/api UniterClient
//go:generate go run github.com/canonical/gomock/mockgen -package mocks -destination mocks/domain_mocks.go github.com/juju/juju/internal/worker/uniter/api Unit,Relation,RelationUnit,Application,Charm
//go:generate go run github.com/canonical/gomock/mockgen -package mocks -destination mocks/secrets_mocks.go github.com/juju/juju/internal/worker/uniter/api SecretsClient,SecretsBackend

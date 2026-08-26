// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package k8s

//go:generate go run github.com/canonical/gomock/mockgen -package k8s -destination resolver_mock_test.go github.com/juju/juju/internal/worker/sshserver/handlers/k8s Resolver

// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package certupdater

//go:generate go run go.uber.org/mock/mockgen -typed -package certupdater -destination package_mock_test.go github.com/juju/juju/internal/worker/certupdater ControllerNodeService,ControllerDomainServices
//go:generate go run go.uber.org/mock/mockgen -typed -package certupdater -destination pki_mock_test.go github.com/juju/juju/internal/pki Authority,LeafRequest

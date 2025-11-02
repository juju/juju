// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelsecrets_test

//go:generate go run go.uber.org/mock/mockgen -package crossmodelsecrets_test -destination package_mock_test.go github.com/juju/juju/apiserver/facades/controller/crossmodelsecrets SecretService,SecretBackendService,CrossModelRelationService
//go:generate go run go.uber.org/mock/mockgen -typed -package crossmodelsecrets_test -destination auth_mock_test.go github.com/juju/juju/apiserver/facade CrossModelAuthContext,MacaroonAuthenticator

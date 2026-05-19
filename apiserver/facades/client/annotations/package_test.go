// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package annotations_test

//go:generate go run github.com/canonical/gomock/mockgen -package annotations -destination annotations_mock_test.go github.com/juju/juju/apiserver/facades/client/annotations AnnotationService
//go:generate go run github.com/canonical/gomock/mockgen -package annotations -destination authorizer_mock_test.go github.com/juju/juju/apiserver/facade Authorizer

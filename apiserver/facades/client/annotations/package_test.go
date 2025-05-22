// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package annotations_test

import (
	stdtesting "testing"

	"github.com/juju/tc"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package annotations -destination annotations_mock_test.go github.com/juju/juju/apiserver/facades/client/annotations AnnotationService
//go:generate go run go.uber.org/mock/mockgen -typed -package annotations -destination authorizer_mock_test.go github.com/juju/juju/apiserver/facade Authorizer


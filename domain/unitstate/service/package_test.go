// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination state_mock_test.go github.com/juju/juju/domain/unitstate/service State
//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination leadership_mock_test.go github.com/juju/juju/core/leadership Ensurer

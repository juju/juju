// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/local_mock.go github.com/juju/juju/apiserver/facades/client/action State,Model
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/state_mock.go github.com/juju/juju/state Action,ActionReceiver

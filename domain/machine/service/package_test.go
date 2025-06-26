// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination state_mock_test.go -source=./service.go
//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination provider_mock_test.go -source=./provider.go
//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination migration_mock_test.go -source=./migration.go

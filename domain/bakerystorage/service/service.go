// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

// State represents a type for interacting with the underlying
// storage required for this service
type State interface {
	BakeryConfigState
}

// Service provides the API for managing the macaroon bakery
// storage
type Service struct {
	*BakeryConfigService
}

// NewService returns a new Service providing an API to manage
// macaroon bakery storage
func NewService(st State) *Service {
	return &Service{
		BakeryConfigService: NewBakeryConfigService(st),
	}
}

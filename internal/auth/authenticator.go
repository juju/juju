// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package auth

import "context"

type AuthenticatedActorType string

const (
	AuthenticatedEntityTypeController AuthenticatedActorType = "controller"
	AuthenticatedEntityTypeMachine    AuthenticatedActorType = "machine"
	AuthenticatedEntityTypeUser       AuthenticatedActorType = "user"
)

// Authenticator is an abstract concept of something that can authenticate a
// singular context between two actors in Juju. The expectation is that given
// two actors representing seperate IPCs in the context of a Juju controller
// they should be able to us an [Authenticator] to establish trust and identity
// between each other.
//
// An Authenticator in this package makes no assumptions to the context of how
// the communication between the two actors occured and leaves this
// implementation detail to the high level communication component. It is
// expected that when a caller receives an [Authenticator] the communication
// context has been captured within the [Authenticator].
//
// An [Authenticator] can only ever be used to establish trust and identity
// between two actors that are the same within the life of a singular context.
// That is should the a new context occur or the current context go out of scope
// this [Authenticator] MUST not be reused. Reusing an [Authenticator] across
// different contexts is undefined behaviour.
//
// Examples of a context for which this might be used could be an API connection,
// unix domain socket or an out of band encrypted message or signal.
//
// Authenticators MUST not be considered safe for concurrent use.
type Authenticator interface {
	// Authenticate is used to validate and verify trust between two actors in
	// Juju. The result of a successful authentication is a non nill
	// [AuthResult] and true.
	//
	// If this [Authenticator] is not capable of establishing trust between the
	// two actors then it will return a nil [AuthResult] and false with a nill
	// error.
	//
	// Should for any reason the authentication fail to be perform an error
	// result will be returned along with a nil [AuthResult] and false.
	//
	// If the supplied context is or becomes cancelled the caller can expect an
	// error result matching [context.Context.Err].
	Authenticate(context.Context) (AuthResult, bool, error)
}

// AuthResult represents an implementation specific explanation about a
// successful [Authenticator.Authenticate] call. It allows the caller
// establishing trust to retrieve more information about the authentication
// result after trust has been established.
//
// AuthResult MUST not be considered safe for concurrent use.
type AuthResult interface {
	// AuthenticatedActor returns the information about the actor that has been
	// authenticated and trust established with. The actor represents the
	// opposing actor to that of the caller using the result.
	//
	// AuthenticatedActor is safe to be called multiple times by a caller with
	// the result being idempotent when no error is present.
	//
	// When a nil error response is provided the [AuthenticatedActorType] will
	// correctly represent the type of actor that has been authenticated along
	// with the unique UUID of the actor within the Juju controller.
	//
	// If the supplied context is or becomes cancelled the caller can expect an
	// error result matching [context.Context.Err].
	AuthenticatedActor(context.Context) (AuthenticatedActorType, string, error)
}

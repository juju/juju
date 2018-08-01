package idmclient

import (
	"gopkg.in/macaroon-bakery.v2-unstable/bakery/checkers"
)

// TODO move these interface declarations into the bakery repository.

// IdentityClient represents an abstract identity manager. A client can
// use this to create a third party caveat requesting authentication and
// to decode the authentication information from the resulting discharge
// macaroon caveats and then to find information about the resulting
// identity.
type IdentityClient interface {
	// IdentityCaveats encodes identity caveats addressed to the identity
	// service that request the service to authenticate the user.
	IdentityCaveats() []checkers.Caveat

	// DeclaredIdentity parses the identity declaration from the given
	// declared attributes.
	DeclaredIdentity(declared map[string]string) (Identity, error)
}

// IdentityCaveatServer represents the server side of the identity
// service caveat contract. An identity server can use this
// to decode third party caveats addressed to it by clients
// and to encode the resulting username information.
type IdentityCaveatServer interface {
}

// Identity holds identity information declared in a first party caveat
// added when discharging a third party caveat.
type Identity interface {
	// Id returns the id of the user, which may be an
	// opaque blob with no human meaning.
	// An id is only considered to be unique
	// with a given domain.
	Id() string

	// Domain holds the domain of the user. This
	// will be empty if the user was authenticated
	// directly with the identity provider.
	Domain() string
}

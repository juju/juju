package httpbakery

import (
	"net/http"
	"time"

	"golang.org/x/net/context"
	errgo "gopkg.in/errgo.v1"

	"gopkg.in/macaroon-bakery.v2/bakery"
	"gopkg.in/macaroon-bakery.v2/bakery/checkers"
)

// Oven is like bakery.Oven except it provides a method for
// translating errors returned by bakery.AuthChecker into
// errors suitable for passing to WriteError.
type Oven struct {
	// Oven holds the bakery Oven used to create
	// new macaroons to put in discharge-required errors.
	*bakery.Oven

	// AuthnExpiry holds the expiry time of macaroons that
	// are created for authentication. As these are generally
	// applicable to all endpoints in an API, this is usually
	// longer than AuthzExpiry. If this is zero, DefaultAuthnExpiry
	// will be used.
	AuthnExpiry time.Duration

	// AuthzExpiry holds the expiry time of macaroons that are
	// created for authorization. As these are generally applicable
	// to specific operations, they generally don't need
	// a long lifespan, so this is usually shorter than AuthnExpiry.
	// If this is zero, DefaultAuthzExpiry will be used.
	AuthzExpiry time.Duration
}

// Default expiry times for macaroons created by Oven.Error.
const (
	DefaultAuthnExpiry = 7 * 24 * time.Hour
	DefaultAuthzExpiry = 5 * time.Minute
)

// Error processes an error as returned from bakery.AuthChecker
// into an error suitable for returning as a response to req
// with WriteError.
//
// Specifically, it translates bakery.ErrPermissionDenied into
// ErrPermissionDenied and bakery.DischargeRequiredError
// into an Error with an ErrDischargeRequired code, using
// oven.Oven to mint the macaroon in it.
func (oven *Oven) Error(ctx context.Context, req *http.Request, err error) error {
	cause := errgo.Cause(err)
	if cause == bakery.ErrPermissionDenied {
		return errgo.WithCausef(err, ErrPermissionDenied, "")
	}
	derr, ok := cause.(*bakery.DischargeRequiredError)
	if !ok {
		return errgo.Mask(err)
	}
	// TODO it's possible to have more than two levels here - think
	// about some naming scheme for the cookies that allows that.
	expiryDuration := oven.AuthzExpiry
	if expiryDuration == 0 {
		expiryDuration = DefaultAuthzExpiry
	}
	cookieName := "authz"
	if derr.ForAuthentication {
		// Authentication macaroons are a bit different, so use
		// a different cookie name so both can be presented together.
		cookieName = "authn"
		expiryDuration = oven.AuthnExpiry
		if expiryDuration == 0 {
			expiryDuration = DefaultAuthnExpiry
		}
	}
	m, err := oven.Oven.NewMacaroon(ctx, RequestVersion(req), derr.Caveats, derr.Ops...)
	if err != nil {
		return errgo.Notef(err, "cannot mint new macaroon")
	}
	if err := m.AddCaveat(ctx, checkers.TimeBeforeCaveat(time.Now().Add(expiryDuration)), nil, nil); err != nil {
		return errgo.Notef(err, "cannot add time-before caveat")
	}
	return NewDischargeRequiredError(DischargeRequiredErrorParams{
		Macaroon:         m,
		CookieNameSuffix: cookieName,
		Request:          req,
	})
}

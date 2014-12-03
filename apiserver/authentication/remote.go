// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package authentication

import (
	"encoding/base64"
	"encoding/json"

	"github.com/juju/errors"
	"github.com/juju/names"
	"gopkg.in/macaroon-bakery.v0/bakery"
	"gopkg.in/macaroon-bakery.v0/bakery/checkers"
	"gopkg.in/macaroon.v1"

	"github.com/juju/juju/state"
)

// RemoteUser represents a remote user known to an external identity provider.
type RemoteUser struct {
	userTag names.UserTag
}

// NewRemoteUser creates a new RemoteUser instance.
func NewRemoteUser(user string) (*RemoteUser, error) {
	if !names.IsValidUser(user) {
		return nil, errors.Errorf("not a valid user: %q", user)
	}
	return &RemoteUser{names.NewUserTag(user)}, nil
}

// Tag implements the state.Entity interface.
func (ru *RemoteUser) Tag() names.Tag {
	return ru.userTag
}

type remoteCredentials struct {
	Primary    *macaroon.Macaroon
	Discharges []*macaroon.Macaroon
}

// RemoteCredentials defines a document struct for serializing macaroons.
type RemoteCredentials struct {
	remoteCredentials
}

// NewRemoteCredentials creates a new RemoteCredentials instance with the given
// primary and discharge macaroons.
func NewRemoteCredentials(primary *macaroon.Macaroon, discharges ...*macaroon.Macaroon) *RemoteCredentials {
	return &RemoteCredentials{
		remoteCredentials{
			Primary:    primary,
			Discharges: discharges,
		},
	}
}

func (rc *RemoteCredentials) macaroons() []*macaroon.Macaroon {
	return append([]*macaroon.Macaroon{rc.Primary}, rc.Discharges...)
}

func (rc *RemoteCredentials) RemoteUser() (*RemoteUser, error) {
	var user string
	for _, m := range rc.macaroons() {
		for _, cav := range m.Caveats() {
			question, arg, err := checkers.ParseCaveat(cav.Id)
			if err != nil {
				return nil, errors.Trace(err)
			}
			if question == "declared-user" {
				if user != "" {
					return nil, errors.New("declared-user conflict")
				}
				user = arg
			}
		}
	}
	if user == "" {
		return nil, errors.New("missing declared-user")
	}
	return NewRemoteUser(user)
}

// Bind binds all discharge macaroons to the primary macaroon.
func (rc *RemoteCredentials) Bind() {
	for _, dm := range rc.Discharges {
		dm.Bind(rc.Primary.Signature())
	}
}

// MarshalText implements the encoding.TextMarshaler interface.
func (rc *RemoteCredentials) MarshalText() ([]byte, error) {
	out, err := json.Marshal(rc.remoteCredentials)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return []byte(base64.URLEncoding.EncodeToString(out)), nil
}

var malformedRemoteCredentialsErr = errors.New("malformed remote credentials")

// IsMalformedRemoteCredentialsErr returns whether the error indicates a
// credentials string was not a well-formed remote credential string.
func IsMalformedRemoteCredentialsErr(err error) bool {
	return errors.Cause(err) == malformedRemoteCredentialsErr
}

// UnmarshalText implements the encoding.TextUnmarshaler interface.
func (rc *RemoteCredentials) UnmarshalText(text []byte) error {
	in, err := base64.URLEncoding.DecodeString(string(text))
	if err != nil {
		return errors.Wrap(err, malformedRemoteCredentialsErr)
	}
	err = json.Unmarshal(in, &rc.remoteCredentials)
	if err != nil {
		return errors.Wrap(err, malformedRemoteCredentialsErr)
	}
	if rc.Primary == nil {
		return errors.New("missing primary credential")
	}
	return nil
}

// AddCredsToRequest adds all macaroons contained in the credential to a
// bakery.Request for target service verification.
func AddCredsToRequest(rc *RemoteCredentials, r *bakery.Request) {
	r.AddClientMacaroon(rc.Primary)
	for _, dm := range rc.Discharges {
		r.AddClientMacaroon(dm)
	}
}

// RemoteAuthenticator authenticates credentials for remote identities.
type RemoteAuthenticator struct {
	*bakery.Service
}

// NewRemoteAuthenticator creates a new instance for authenticating credentials
// from the perspective of the given target service.
func NewRemoteAuthenticator(service *bakery.Service) *RemoteAuthenticator {
	return &RemoteAuthenticator{Service: service}
}

func declaredUserChecker(user string) bakery.FirstPartyCheckerFunc {
	return func(cav string) error {
		question, arg, err := checkers.ParseCaveat(cav)
		if err != nil {
			return err
		}
		if question == "declared-user" {
			if arg == user {
				return nil
			}
			return errors.Errorf("invalid user")
		}
		return &bakery.CaveatNotRecognizedError{cav}
	}
}

func caveatsIncludeChecker(remoteCreds *RemoteCredentials, question string) bakery.FirstPartyCheckerFunc {
	return func(cav string) error {
		question, arg, err := checkers.ParseCaveat(cav)
		if err != nil {
			return err
		}
		if question == "caveats-include" {
			for _, m := range remoteCreds.macaroons() {
				for _, matchCaveat := range m.Caveats() {
					cavQuestion, _, err := checkers.ParseCaveat(matchCaveat.Id)
					if err != nil {
						return nil
					}
					if cavQuestion == arg {
						return nil
					}
				}
			}
			return errors.Errorf("missing required caveat")
		}
		return &bakery.CaveatNotRecognizedError{cav}
	}
}

// Authenticate returns the remote user declared by the credentials, and
// whether they are valid.
func (a *RemoteAuthenticator) Authenticate(credential string) (state.Entity, error) {
	var remoteCreds RemoteCredentials
	err := remoteCreds.UnmarshalText([]byte(credential))
	if err != nil {
		return nil, errors.Trace(err)
	}

	remoteUser, err := remoteCreds.RemoteUser()
	if err != nil {
		return nil, errors.Trace(err)
	}

	firstPartyChecker := checkers.PushFirstPartyChecker(checkers.Std, checkers.Map{
		"declared-user":   declaredUserChecker(remoteUser.userTag.Id()),
		"caveats-include": caveatsIncludeChecker(&remoteCreds, "declared-user"),
	})

	r := a.NewRequest(firstPartyChecker)
	AddCredsToRequest(&remoteCreds, r)
	err = r.Check()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return remoteUser, nil
}

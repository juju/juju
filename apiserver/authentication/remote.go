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

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/state"
)

// RemoteUser represents a remote user known to an external identity provider.
type RemoteUser struct {
	userTag names.UserTag
}

// NewRemoteUser creates a new RemoteUser instance.
func NewRemoteUser(userName string) (*RemoteUser, error) {
	userTag, err := names.ParseUserTag(userName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &RemoteUser{userTag}, nil
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

// Authenticate implements the EntityAuthenticator interface.
func (a *RemoteAuthenticator) Authenticate(entity state.Entity, credential, _ string) error {
	remoteUser, ok := entity.(*RemoteUser)
	if !ok {
		logger.Infof("not a remote user: %q", entity)
		return common.ErrBadCreds
	}

	var remoteCreds RemoteCredentials
	err := remoteCreds.UnmarshalText([]byte(credential))
	if err != nil {
		return errors.Trace(err)
	}

	firstPartyChecker := checkers.PushFirstPartyChecker(checkers.Std, checkers.Map{
		"declared-user": declaredUserChecker(remoteUser.userTag.Id()),
	})
	r := a.NewRequest(firstPartyChecker)
	AddCredsToRequest(&remoteCreds, r)
	return r.Check()
}

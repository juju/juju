// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package authentication

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/macaroon"
	"github.com/juju/macaroon/bakery"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/state"
)

// RemoteUser represents a remote user defined by an external identity
// provider.
type RemoteUser struct {
	authTag   names.UserTag
	sessionId string
}

// NewRemoteUser creates a new RemoteUser instance.
func NewRemoteUser(authTag, sessionId string) (*RemoteUser, error) {
	tag, err := names.ParseTag(authTag)
	if err != nil {
		return nil, err
	}
	switch tag := tag.(type) {
	case names.UserTag:
		return &RemoteUser{
			authTag:   tag,
			sessionId: sessionId,
		}, nil
	default:
		return nil, errors.Errorf("not a remote user tag: %q", tag)
	}
}

// Tag implements the names.Tag interface.
func (ru *RemoteUser) Tag() names.Tag {
	return ru.authTag
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
		return nil, err
	}
	return []byte(base64.URLEncoding.EncodeToString(out)), nil
}

var malformedRemoteCredentialsErr = errors.Errorf("malformed remote credentials")

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

// CheckFirstPartyCaveat implements the bakery.FirstPartyChecker interface.
func (*RemoteAuthenticator) CheckFirstPartyCaveat(caveat string) error {
	fields := strings.Split(caveat, " ")
	if len(fields) < 1 {
		return errors.Errorf("empty caveat")
	}
	switch fields[0] {
	case "is-before-time?":
		if len(fields) < 2 {
			return errors.Errorf("%s: missing expiration", fields[0])
		}
		expiresAt, err := time.Parse(time.RFC3339, fields[1])
		if err != nil {
			return errors.Errorf("%s: invalid expiration %s", fields[0], fields[1])
		}
		if time.Now().UTC().After(expiresAt) {
			return errors.Errorf("%s: authorization expired at %s", fields[0], fields[1])
		}
	}
	return nil
}

// Authenticate implements the EntityAuthenticator interface.
func (a *RemoteAuthenticator) Authenticate(entity state.Entity, credential, nonce string) error {
	remoteUser, ok := entity.(*RemoteUser)
	if !ok {
		logger.Infof("not a remote user: %q", entity)
		return common.ErrBadCreds
	}
	// TODO (cmars): necessary to check this?
	if remoteUser.sessionId != nonce {
		logger.Infof("remote user session %q does not match nonce %q", remoteUser.sessionId, nonce)
		return common.ErrBadCreds
	}

	var remoteCreds RemoteCredentials
	err := remoteCreds.UnmarshalText([]byte(credential))
	if err != nil {
		return err
	}
	if remoteCreds.Primary.Id() != nonce {
		logger.Infof("invalid credential")
		return common.ErrBadCreds
	}

	r := a.NewRequest(a)
	AddCredsToRequest(&remoteCreds, r)
	return r.Check()
}

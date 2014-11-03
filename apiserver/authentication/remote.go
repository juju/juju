// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package authentication

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/juju/macaroon"
	"github.com/juju/macaroon/bakery"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/state"
)

type RemoteUser struct {
	authTag   names.UserTag
	sessionId string
}

var _ state.Entity = (*RemoteUser)(nil)

func NewRemoteUser(authTag, sessionId string) (*RemoteUser, error) {
	tag, err := names.ParseTag(authTag)
	if err != nil {
		return nil, err
	}
	userTag, ok := tag.(names.UserTag)
	if !ok {
		return nil, fmt.Errorf("not a remote user tag: %q", tag)
	}
	return &RemoteUser{
		authTag:   userTag,
		sessionId: sessionId,
	}, nil
}

func (ru *RemoteUser) Tag() names.Tag {
	return ru.authTag
}

type remoteCredentials struct {
	Primary    *macaroon.Macaroon
	Discharges []*macaroon.Macaroon
}

type RemoteCredentials struct {
	remoteCredentials
}

func NewRemoteCredentials(primary *macaroon.Macaroon, discharges ...*macaroon.Macaroon) *RemoteCredentials {
	return &RemoteCredentials{
		remoteCredentials{
			Primary:    primary,
			Discharges: discharges,
		},
	}
}

func (rc *RemoteCredentials) MarshalText() ([]byte, error) {
	out, err := json.Marshal(rc.remoteCredentials)
	if err != nil {
		return nil, err
	}
	return []byte(base64.URLEncoding.EncodeToString(out)), nil
}

func (rc *RemoteCredentials) UnmarshalText(text []byte) error {
	in, err := base64.URLEncoding.DecodeString(string(text))
	if err != nil {
		return err
	}
	err = json.Unmarshal(in, &rc.remoteCredentials)
	if err != nil {
		return err
	}
	if rc.Primary == nil {
		return fmt.Errorf("missing primary credential")
	}
	return nil
}

func (rc *RemoteCredentials) AddToRequest(r *bakery.Request) {
	r.AddClientMacaroon(rc.Primary)
	for _, dm := range rc.Discharges {
		r.AddClientMacaroon(dm)
	}
}

type RemoteAuthenticator struct {
	*bakery.Service
}

var _ EntityAuthenticator = (*RemoteAuthenticator)(nil)

func NewRemoteAuthenticator(service *bakery.Service) *RemoteAuthenticator {
	return &RemoteAuthenticator{Service: service}
}

func (*RemoteAuthenticator) CheckFirstPartyCaveat(caveat string) error {
	// TODO (cmars): what first party caveats does the Juju server need to
	// support?
	return nil
}

func (a *RemoteAuthenticator) Authenticate(entity state.Entity, credential, nonce string) error {
	remoteUser, ok := entity.(*RemoteUser)
	if !ok {
		logger.Debugf("not a remote user: %q", entity)
		return common.ErrBadCreds
	}
	// TODO (cmars): necessary to check this?
	if remoteUser.sessionId != nonce {
		logger.Debugf("remote user session %q does not match nonce %q", remoteUser.sessionId, nonce)
		return common.ErrBadCreds
	}

	var remoteCreds RemoteCredentials
	err := remoteCreds.UnmarshalText([]byte(credential))
	if err != nil {
		return err
	}
	if remoteCreds.Primary.Id() != nonce {
		logger.Debugf("invalid credential")
		return common.ErrBadCreds
	}

	r := a.NewRequest(a)
	remoteCreds.AddToRequest(r)
	return r.Check()
}

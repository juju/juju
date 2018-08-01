// Copyright 2014 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package params

import (
	"bytes"
	"unicode/utf8"

	"github.com/juju/httprequest"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon-bakery.v2-unstable/bakery"
	"gopkg.in/macaroon.v2-unstable"
)

// Username represents the name of a user.
type Username string

// UnmarshalText unmarshals a Username checking it is valid. It
// implements "encoding".TextUnmarshaler.
func (u *Username) UnmarshalText(b []byte) error {
	if utf8.RuneCount(b) > 256 {
		return errgo.New("username longer than 256 characters")
	}
	for _, part := range bytes.Split(b, []byte("@")) {
		if !names.IsValidUserName(string(part)) {
			return errgo.Newf("illegal username %q", b)
		}
	}
	*u = Username(string(b))
	return nil
}

// AgentLogin contains the claimed identity the agent is attempting to
// use to log in.
type AgentLogin struct {
	Username  Username          `json:"username"`
	PublicKey *bakery.PublicKey `json:"public_key"`
}

// AgentLoginResponse contains the response to an agent login attempt.
type AgentLoginResponse struct {
	AgentLogin bool `json:"agent_login"`
}

// PublicKeyRequest documents the /publickey endpoint. As
// it contains no request information there is no need to ever create
// one.
type PublicKeyRequest struct {
	httprequest.Route `httprequest:"GET /publickey"`
}

// PublicKeyResponse is the response to a PublicKeyRequest.
type PublicKeyResponse struct {
	PublicKey *bakery.PublicKey
}

// LoginMethods holds the response from the /v1/login endpoint
// when called with "Accept: application/json". This enumerates
// the available methods for the client to log in.
type LoginMethods struct {
	// Agent is the endpoint to connect to, if the client wishes to
	// authenticate as an agent.
	Agent string `json:"agent,omitempty"`

	// Interactive is the endpoint to connect to, if the user can
	// interact with the login process.
	Interactive string `json:"interactive,omitempty"`

	// UbuntuSSO OAuth is the endpoint to send a request, signed with
	// UbuntuSSO OAuth credentials, to if the client wishes to use
	// oauth to log in to Identity Manager. Ubuntu SSO uses oauth 1.0.
	UbuntuSSOOAuth string `json:"usso_oauth,omitempty"`

	// Form is the endpoint to GET a schema for a login form which
	// can be presented to the user in an interactive manner. The
	// schema will be returned as an environschema.Fields object. The
	// completed form should be POSTed back to the same endpoint.
	Form string `json:"form,omitempty"`
}

// QueryUsersRequest is a request to query the users in the system.
type QueryUsersRequest struct {
	httprequest.Route `httprequest:"GET /v1/u" bson:",omitempty"`
	ExternalID        string `httprequest:"external_id,form" bson:"external_id,omitempty"`
}

// UserRequest is a request for the user details of the named user.
type UserRequest struct {
	httprequest.Route `httprequest:"GET /v1/u/:username"`
	Username          Username `httprequest:"username,path"`
}

// User represents a user in the system.
type User struct {
	Username   Username            `json:"username,omitempty"`
	ExternalID string              `json:"external_id"`
	FullName   string              `json:"fullname"`
	Email      string              `json:"email"`
	GravatarID string              `json:"gravatar_id"`
	IDPGroups  []string            `json:"idpgroups"`
	Owner      Username            `json:"owner,omitempty"`
	PublicKeys []*bakery.PublicKey `json:"public_keys"`
	SSHKeys    []string            `json:"ssh_keys"`
}

// SetUserRequest is request to set the details of a user.
type SetUserRequest struct {
	httprequest.Route `httprequest:"PUT /v1/u/:username"`
	Username          Username `httprequest:"username,path"`
	User              `httprequest:",body"`
}

// UserGroupsRequest is a request for the list of groups associated
// with the specified user.
type UserGroupsRequest struct {
	httprequest.Route `httprequest:"GET /v1/u/:username/groups"`
	Username          Username `httprequest:"username,path"`
}

// UserIDPGroupsRequest defines the deprecated path for
// UserGroupsRequest. It should no longer be used.
type UserIDPGroupsRequest struct {
	httprequest.Route `httprequest:"GET /v1/u/:username/idpgroups"`
	UserGroupsRequest
}

// UserTokenRequest is a request for a new token to represent the user.
type UserTokenRequest struct {
	httprequest.Route `httprequest:"GET /v1/u/:username/macaroon"`
	Username          Username `httprequest:"username,path"`
}

// VerifyTokenRequest is a request to verify that the provided
// macaroon.Slice is valid and represents a user from identity.
type VerifyTokenRequest struct {
	httprequest.Route `httprequest:"POST /v1/verify"`
	Macaroons         macaroon.Slice `httprequest:",body"`
}

// SSHKeysRequest is a request for the list of ssh keys associated
// with the specified user.
type SSHKeysRequest struct {
	httprequest.Route `httprequest:"GET /v1/u/:username/ssh-keys"`
	Username          Username `httprequest:"username,path"`
}

// UserSSHKeysResponse holds a response to the GET /v1/u/:username/ssh-keys
// containing list of ssh keys associated with the user.
type SSHKeysResponse struct {
	SSHKeys []string `json:"ssh_keys"`
}

// PutSSHKeysRequest is a request to set ssh keys to the list of ssh keys
// associated with the user.
type PutSSHKeysRequest struct {
	httprequest.Route `httprequest:"PUT /v1/u/:username/ssh-keys"`
	Username          Username       `httprequest:"username,path"`
	Body              PutSSHKeysBody `httprequest:",body"`
}

// PutSSHKeysBody holds the body of a PutSSHKeysRequest.
type PutSSHKeysBody struct {
	SSHKeys []string `json:"ssh-keys"`
	Add     bool     `json:"add,omitempty"`
}

// DeleteSSHKeysRequest is a request to remove ssh keys from the list of ssh keys
// associated with the user.
type DeleteSSHKeysRequest struct {
	httprequest.Route `httprequest:"DELETE /v1/u/:username/ssh-keys"`
	Username          Username          `httprequest:"username,path"`
	Body              DeleteSSHKeysBody `httprequest:",body"`
}

// DeleteSSHKeysBody holds the body of a DeleteSSHKeysRequest.
type DeleteSSHKeysBody struct {
	SSHKeys []string `json:"ssh-keys"`
}

// UserExtraInfoRequest is a request for the arbitrary extra information
// stored about the user.
type UserExtraInfoRequest struct {
	httprequest.Route `httprequest:"GET /v1/u/:username/extra-info"`
	Username          Username `httprequest:"username,path"`
}

// SetUserExtraInfoRequest is a request to updated the arbitrary extra
// information stored about the user.
type SetUserExtraInfoRequest struct {
	httprequest.Route `httprequest:"PUT /v1/u/:username/extra-info"`
	Username          Username               `httprequest:"username,path"`
	ExtraInfo         map[string]interface{} `httprequest:",body"`
}

// UserExtraInfoItemRequest is a request for a single element of the
// arbitrary extra information stored about the user.
type UserExtraInfoItemRequest struct {
	httprequest.Route `httprequest:"GET /v1/u/:username/extra-info/:item"`
	Username          Username `httprequest:"username,path"`
	Item              string   `httprequest:"item,path"`
}

// SetUserExtraInfoItemRequest is a request to update a single element of
// the arbitrary extra information stored about the user.
type SetUserExtraInfoItemRequest struct {
	httprequest.Route `httprequest:"PUT /v1/u/:username/extra-info/:item"`
	Username          Username    `httprequest:"username,path"`
	Item              string      `httprequest:"item,path"`
	Data              interface{} `httprequest:",body"`
}

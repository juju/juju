// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

// Cloud holds information about a cloud.
type Cloud struct {
	Type            string        `json:"type"`
	AuthTypes       []string      `json:"auth-types,omitempty"`
	Endpoint        string        `json:"endpoint,omitempty"`
	StorageEndpoint string        `json:"endpoint,omitempty"`
	Regions         []CloudRegion `json:"regions,omitempty"`
}

// CloudRegion holds information about a cloud region.
type CloudRegion struct {
	Name            string `json:"name"`
	Endpoint        string `json:"endpoint,omitempty"`
	StorageEndpoint string `json:"endpoint,omitempty"`
}

// CloudCredential contains a cloud credential.
type CloudCredential struct {
	AuthType   string            `json:"auth-type"`
	Attributes map[string]string `json:"attrs,omitempty"`
}

type CloudCredentialsResult struct {
	Error       *Error                     `json:"error,omitempty"`
	Credentials map[string]CloudCredential `json:"credentials,omitempty"`
}

type CloudCredentialsResults struct {
	Results []CloudCredentialsResult `json:"results,omitempty"`
}

type UserCloudCredentials struct {
	UserTag     string                     `json:"user-tag"`
	Credentials map[string]CloudCredential `json:"credentials"`
}

type UsersCloudCredentials struct {
	Users []UserCloudCredentials `json:"users"`
}

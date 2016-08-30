// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

// Cloud holds information about a cloud.
type Cloud struct {
	Type             string        `json:"type"`
	AuthTypes        []string      `json:"auth-types,omitempty"`
	Endpoint         string        `json:"endpoint,omitempty"`
	IdentityEndpoint string        `json:"identity-endpoint,omitempty"`
	StorageEndpoint  string        `json:"storage-endpoint,omitempty"`
	Regions          []CloudRegion `json:"regions,omitempty"`
}

// CloudRegion holds information about a cloud region.
type CloudRegion struct {
	Name             string `json:"name"`
	Endpoint         string `json:"endpoint,omitempty"`
	IdentityEndpoint string `json:"identity-endpoint,omitempty"`
	StorageEndpoint  string `json:"storage-endpoint,omitempty"`
}

// CloudResult contains a cloud definition or an error.
type CloudResult struct {
	Cloud *Cloud `json:"cloud,omitempty"`
	Error *Error `json:"error,omitempty"`
}

// CloudResults contains a set of CloudResults.
type CloudResults struct {
	Results []CloudResult `json:"results,omitempty"`
}

// CloudsResult contains a set of Clouds.
type CloudsResult struct {
	// Clouds is a map of clouds, keyed by cloud tag.
	Clouds map[string]Cloud `json:"clouds,omitempty"`
}

// CloudCredential contains a cloud credential
// possibly with secrets redacted.
type CloudCredential struct {
	// AuthType is the authentication type.
	AuthType string `json:"auth-type"`

	// Attributes contains non-secret credential values.
	Attributes map[string]string `json:"attrs,omitempty"`

	// Redacted is a list of redacted attributes
	Redacted []string `json:"redacted,omitempty"`
}

// CloudCredentialResult contains a CloudCredential or an error.
type CloudCredentialResult struct {
	Result *CloudCredential `json:"result,omitempty"`
	Error  *Error           `json:"error,omitempty"`
}

// CloudCredentialResults contains a set of CloudCredentialResults.
type CloudCredentialResults struct {
	Results []CloudCredentialResult `json:"results,omitempty"`
}

// UserCloud contains a user/cloud tag pair, typically used for identifying
// a user's credentials for a cloud.
type UserCloud struct {
	UserTag  string `json:"user-tag"`
	CloudTag string `json:"cloud-tag"`
}

// UserClouds contains a set of UserClouds.
type UserClouds struct {
	UserClouds []UserCloud `json:"user-clouds,omitempty"`
}

// UpdateCloudCredentials contains a set of tagged cloud credentials.
type UpdateCloudCredentials struct {
	Credentials []UpdateCloudCredential `json:"credentials,omitempty"`
}

// UpdateCloudCredential contains a cloud credential and its tag,
// for updating in state.
type UpdateCloudCredential struct {
	Tag        string          `json:"tag"`
	Credential CloudCredential `json:"credential"`
}

// CloudSpec holds a cloud specification.
type CloudSpec struct {
	Type             string           `json:"type"`
	Name             string           `json:"name"`
	Region           string           `json:"region,omitempty"`
	Endpoint         string           `json:"endpoint,omitempty"`
	IdentityEndpoint string           `json:"identity-endpoint,omitempty"`
	StorageEndpoint  string           `json:"storage-endpoint,omitempty"`
	Credential       *CloudCredential `json:"credential,omitempty"`
}

// CloudSpecResult contains a CloudSpec or an error.
type CloudSpecResult struct {
	Result *CloudSpec `json:"result,omitempty"`
	Error  *Error     `json:"error,omitempty"`
}

// CloudSpecResults contains a set of CloudSpecResults.
type CloudSpecResults struct {
	Results []CloudSpecResult `json:"results,omitempty"`
}

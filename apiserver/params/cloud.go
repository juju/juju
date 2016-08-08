// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

// Cloud holds information about a cloud.
type Cloud struct {
	Type            string        `json:"type"`
	AuthTypes       []string      `json:"auth-types,omitempty"`
	Endpoint        string        `json:"endpoint,omitempty"`
	StorageEndpoint string        `json:"storage-endpoint,omitempty"`
	Regions         []CloudRegion `json:"regions,omitempty"`
}

// CloudRegion holds information about a cloud region.
type CloudRegion struct {
	Name            string `json:"name"`
	Endpoint        string `json:"endpoint,omitempty"`
	StorageEndpoint string `json:"storage-endpoint,omitempty"`
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

// CloudCredential contains a cloud credential.
type CloudCredential struct {
	AuthType   string            `json:"auth-type"`
	Attributes map[string]string `json:"attrs,omitempty"`
}

// CloudCredentialsResult contains a set of credentials for a user and cloud,
// or an error.
type CloudCredentialsResult struct {
	Error       *Error                     `json:"error,omitempty"`
	Credentials map[string]CloudCredential `json:"credentials,omitempty"`
}

// CloudCredentialsResults contains a set of CloudCredentialsResults.
type CloudCredentialsResults struct {
	Results []CloudCredentialsResult `json:"results,omitempty"`
}

// UserCloud contains a user/cloud tag pair, typically used for identifying
// a user's credentials for a cloud.
type UserCloud struct {
	UserTag  string `json:"user-tag"`
	CloudTag string `json:"cloud-tag"`
}

// UserClouds contains a set of USerClouds.
type UserClouds struct {
	UserClouds []UserCloud `json:"user-clouds,omitempty"`
}

// UserCloudCredentials contains a user's credentials for a cloud.
type UserCloudCredentials struct {
	UserTag     string                     `json:"user-tag"`
	CloudTag    string                     `json:"cloud-tag"`
	Credentials map[string]CloudCredential `json:"credentials"`
}

// UsersCloudCredentials contains a set of UserCloudCredentials.
type UsersCloudCredentials struct {
	Users []UserCloudCredentials `json:"users"`
}

// CloudDefaults contains defaults for cloud name, region, and
// credential for a user.
type CloudDefaults struct {
	CloudTag        string `json:"cloud-tag"`
	CloudRegion     string `json:"region,omitempty"`
	CloudCredential string `json:"credential,omitempty"`
}

// CloudDefaultsResult contains a CloudDefaults or an error.
type CloudDefaultsResult struct {
	Result *CloudDefaults `json:"result,omitempty"`
	Error  *Error         `json:"error,omitempty"`
}

// CloudDefaultsResults contains a set of CloudDefaultsResults.
type CloudDefaultsResults struct {
	Results []CloudDefaultsResult `json:"results,omitempty"`
}

// CloudSpec holds a cloud specification.
type CloudSpec struct {
	Type            string           `json:"type"`
	Name            string           `json:"name"`
	Region          string           `json:"region,omitempty"`
	Endpoint        string           `json:"endpoint,omitempty"`
	StorageEndpoint string           `json:"storage-endpoint,omitempty"`
	Credential      *CloudCredential `json:"credential,omitempty"`
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

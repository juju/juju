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
	CACertificates   []string      `json:"ca-certificates,omitempty"`
}

// CloudRegion holds information about a cloud region.
type CloudRegion struct {
	Name             string `json:"name"`
	Endpoint         string `json:"endpoint,omitempty"`
	IdentityEndpoint string `json:"identity-endpoint,omitempty"`
	StorageEndpoint  string `json:"storage-endpoint,omitempty"`
}

// AddCloudArgs holds a cloud to be added with its name
type AddCloudArgs struct {
	Cloud Cloud  `json:"cloud"`
	Name  string `json:"name"`
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

// CloudUserInfo holds information on a user who has access to a
// cloud. Cloud admins can see this information for all users
// who have access, so it should not include sensitive information.
type CloudUserInfo struct {
	UserName    string `json:"user"`
	DisplayName string `json:"display-name"`
	Access      string `json:"access"`
}

// CloudDetails holds information about a cloud.
type CloudDetails struct {
	Type             string        `json:"type"`
	AuthTypes        []string      `json:"auth-types,omitempty"`
	Endpoint         string        `json:"endpoint,omitempty"`
	IdentityEndpoint string        `json:"identity-endpoint,omitempty"`
	StorageEndpoint  string        `json:"storage-endpoint,omitempty"`
	Regions          []CloudRegion `json:"regions,omitempty"`
}

// CloudInfo holds information about a cloud and user who can access it.
type CloudInfo struct {
	CloudDetails `json:",inline"`

	// Users contains information about the users that have access
	// to the cloud. Administrators can see all users that have access;
	// other users can only see their own details.
	Users []CloudUserInfo `json:"users"`
}

// CloudInfoResult holds the result of a CloudInfo call.
type CloudInfoResult struct {
	Result *CloudInfo `json:"result,omitempty"`
	Error  *Error     `json:"error,omitempty"`
}

// CloudInfoResults holds the result of a bulk CloudInfo call.
type CloudInfoResults struct {
	Results []CloudInfoResult `json:"results"`
}

// ListCloudsRequest encapsulates how we request a list of cloud details for a user.
type ListCloudsRequest struct {
	UserTag string `json:"user-tag"`
	All     bool   `json:"all,omitempty"`
}

// ListCloudInfo holds information about a cloud for a user.
type ListCloudInfo struct {
	CloudDetails `json:",inline"`

	// Access is the access level for the user.
	Access string `json:"user-access"`
}

// ListCloudInfoResult holds the result of a ListCloudInfo call.
type ListCloudInfoResult struct {
	Result *ListCloudInfo `json:"result,omitempty"`
	Error  *Error         `json:"error,omitempty"`
}

// ListCloudInfoResults holds the result of a bulk ListCloudInfo call.
type ListCloudInfoResults struct {
	Results []ListCloudInfoResult `json:"results"`
}

// ModifyCloudAccessRequest holds the parameters for making grant and revoke cloud calls.
type ModifyCloudAccessRequest struct {
	Changes []ModifyCloudAccess `json:"changes"`
}

// ModifyCloudAccess defines an operation to modify cloud access.
type ModifyCloudAccess struct {
	UserTag  string      `json:"user-tag"`
	CloudTag string      `json:"cloud-tag"`
	Action   CloudAction `json:"action"`
	Access   string      `json:"access"`
}

// CloudAction is an action that can be performed on a cloud.
type CloudAction string

// Actions that can be preformed on a cloud.
const (
	GrantCloudAccess  CloudAction = "grant"
	RevokeCloudAccess CloudAction = "revoke"
)

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

// TaggedCredentials contains a set of tagged cloud credentials.
type TaggedCredentials struct {
	Credentials []TaggedCredential `json:"credentials,omitempty"`
}

// TaggedCredential contains a cloud credential and its tag.
type TaggedCredential struct {
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
	CACertificates   []string         `json:"cacertificates,omitempty"`
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

// CloudCredentialArg defines a credential in terms of its cloud and name.
// It is used to request detailed content for the credential stored on the controller.
type CloudCredentialArg struct {
	CloudName      string `json:"cloud-name"`
	CredentialName string `json:"credential-name"`
}

// IsEmpty returns whether a cloud credential argument is empty.
func (p CloudCredentialArg) IsEmpty() bool {
	return p.CloudName == "" && p.CredentialName == ""
}

// CloudCredentialArgs defines an input required to make a valid call
// to get credentials content stored on the controller.
type CloudCredentialArgs struct {
	Credentials    []CloudCredentialArg `json:"credentials,omitempty"`
	IncludeSecrets bool                 `json:"include-secrets"`
}

// CloudCredential contains a cloud credential content.
type CredentialContent struct {
	// Name is the short name of the credential.
	Name string `json:"name"`

	// Cloud is the cloud name to which this credential belongs.
	Cloud string `json:"cloud"`

	// AuthType is the authentication type.
	AuthType string `json:"auth-type"`

	// Attributes contains credential values.
	Attributes map[string]string `json:"attrs,omitempty"`
}

// ModelAccess contains information about user model access.
type ModelAccess struct {
	Model  string `json:"model,omitempty"`
	Access string `json:"access,omitempty"`
}

// ControllerCredentialInfo contains everything Juju stores on the controller
// about the credential - its contents as well as what models use it and
// what access currently logged in user, a credential owner, has to these models.
type ControllerCredentialInfo struct {
	// Content has comprehensive credential content.
	Content CredentialContent `json:"content,omitempty"`

	// Models contains models that are using ths credential.
	Models []ModelAccess `json:"models,omitempty"`
}

// CredentialContentResult contains comprehensive information about stored credential or an error.
type CredentialContentResult struct {
	Result *ControllerCredentialInfo `json:"result,omitempty"`
	Error  *Error                    `json:"error,omitempty"`
}

// CredentialContentResults contains a set of CredentialContentResults.
type CredentialContentResults struct {
	Results []CredentialContentResult `json:"results,omitempty"`
}

// ValidateCredentialArg contains collection of cloud credentials
// identified by their tags to mark as valid or not.
type ValidateCredentialArg struct {
	CredentialTag string `json:"tag"`
	Valid         bool   `json:"valid"`
	Reason        string `json:"reason,omitempty"`
}

// ValidateCredentialArgs contains a set of ValidateCredentialArg.
type ValidateCredentialArgs struct {
	All []ValidateCredentialArg `json:"credentials,omitempty"`
}

// UpdateCredentialModelResult contains results for a model credential validation check
// from a cloud credential update.
type UpdateCredentialModelResult struct {

	// ModelUUID contains model's UUID.
	ModelUUID string `json:"uuid"`

	// ModelName contains model name.
	ModelName string `json:"name"`

	// Errors contains the errors accumulated while trying to update a credential.
	Errors []ErrorResult `json:"errors,omitempty"`
}

// UpdateCredentialResult stores the result of updating one cloud credential.
type UpdateCredentialResult struct {
	// CredentialTag holds credential tag.
	CredentialTag string `json:"tag"`

	// Errors contains an error that may have occurred while trying to update a credential.
	Error *Error `json:"error,omitempty"`

	// Models contains results of credential check against models that use this cloud credential.
	Models []UpdateCredentialModelResult `json:"models,omitempty"`
}

// UpdateCredentialResult contains a set of UpdateCredentialResult.
type UpdateCredentialResults struct {
	Results []UpdateCredentialResult `json:"results,omitempty"`
}

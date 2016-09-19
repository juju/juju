// This file is based on code from Azure/azure-sdk-for-go,
// which is Copyright Microsoft Corporation. See the LICENSE
// file in this directory for details.
//
// NOTE(axw) this file contains types for a subset of the
// Microsoft Graph API, which is not currently supported by
// the Azure SDK. When it is, this will be deleted.

package ad

import (
	"time"

	"github.com/Azure/go-autorest/autorest"
)

type AADObject struct {
	autorest.Response     `json:"-"`
	ObjectID              string   `json:"objectId"`
	ObjectType            string   `json:"objectType"`
	DisplayName           string   `json:"displayName"`
	UserPrincipalName     string   `json:"userPrincipalName"`
	Mail                  string   `json:"mail"`
	MailEnabled           bool     `json:"mailEnabled"`
	SecurityEnabled       bool     `json:"securityEnabled"`
	SignInName            string   `json:"signInName"`
	ServicePrincipalNames []string `json:"servicePrincipalNames"`
	UserType              string   `json:"userType"`
}

type PasswordCredentialsListResult struct {
	autorest.Response `json:"-"`
	Value             []PasswordCredential `json:"value,omitempty"`
}

type PasswordCredentialsUpdateParameters struct {
	Value []PasswordCredential `json:"value,omitempty"`
}

type PasswordCredential struct {
	CustomKeyIdentifier []byte    `json:"customKeyIdentifier,omitempty"`
	KeyId               string    `json:"keyId,omitempty"`
	Value               string    `json:"value,omitempty"`
	StartDate           time.Time `json:"startDate,omitempty"`
	EndDate             time.Time `json:"endDate,omitempty"`
}

type ServicePrincipalListResult struct {
	autorest.Response `json:"-"`
	Value             []ServicePrincipal `json:"value,omitempty"`
}

type ServicePrincipalCreateParameters struct {
	ApplicationID       string               `json:"appId,omitempty"`
	AccountEnabled      bool                 `json:"accountEnabled,omitempty"`
	PasswordCredentials []PasswordCredential `json:"passwordCredentials,omitempty"`
}

type ServicePrincipal struct {
	autorest.Response `json:"-"`
	ApplicationID     string `json:"appId,omitempty"`
	ObjectID          string `json:"objectId,omitempty"`
	AccountEnabled    bool   `json:"accountEnabled,omitempty"`
}

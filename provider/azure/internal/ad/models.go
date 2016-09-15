// NOTE(axw) this file contains types for a subset of the
// Microsoft Graph API, which is not currently supported by
// the Azure SDK. When it is, this will be deleted.

package ad

import (
	"time"

	"github.com/Azure/go-autorest/autorest"
)

type ApplicationListResult struct {
	autorest.Response `json:"-"`
	Value             []Application `json:"value,omitempty"`
}

type Application struct {
	autorest.Response       `json:"-"`
	ApplicationID           string               `json:"appId,omitempty"`
	ObjectID                string               `json:"objectId,omitempty"`
	AvailableToOtherTenants *bool                `json:"availableToOtherTenants,omitempty"`
	DisplayName             string               `json:"displayName,omitempty"`
	Homepage                string               `json:"homepage,omitempty"`
	IdentifierURIs          []string             `json:"identifierUris,omitempty"`
	PasswordCredentials     []PasswordCredential `json:"passwordCredentials,omitempty"`
}

type PasswordCredential struct {
	KeyId     string    `json:"keyId,omitempty"`
	Value     string    `json:"value,omitempty"`
	StartDate time.Time `json:"startDate,omitempty"`
	EndDate   time.Time `json:"endDate,omitempty"`
}

type ServicePrincipalListResult struct {
	autorest.Response `json:"-"`
	Value             []ServicePrincipal `json:"value,omitempty"`
}

type ServicePrincipal struct {
	ApplicationID  string `json:"appId,omitempty"`
	ObjectID       string `json:"objectId,omitempty"`
	AccountEnabled bool   `json:"accountEnabled,omitempty"`
}

// NOTE(axw) this file contains a client for a subset of the
// Microsoft Graph API, which is not currently supported by
// the Azure SDK. When it is, this will be deleted.

package ad

import (
	"github.com/Azure/go-autorest/autorest"
	"github.com/juju/juju/version"
)

const (
	// APIVersion is the version of the Active Directory API
	APIVersion = "1.6"
)

func UserAgent() string {
	return "Juju/" + version.Current.String()
}

type ManagementClient struct {
	autorest.Client
	BaseURI    string
	APIVersion string
}

func NewManagementClient(baseURI string) ManagementClient {
	return ManagementClient{
		Client:     autorest.NewClientWithUserAgent(UserAgent()),
		BaseURI:    baseURI,
		APIVersion: APIVersion,
	}
}

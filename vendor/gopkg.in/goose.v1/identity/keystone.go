package identity

import (
	"fmt"

	goosehttp "gopkg.in/goose.v1/http"
)

type endpoint struct {
	AdminURL    string `json:"adminURL"`
	InternalURL string `json:"internalURL"`
	PublicURL   string `json:"publicURL"`
	Region      string `json:"region"`
}

type serviceResponse struct {
	Name      string `json:"name"`
	Type      string `json:"type"`
	Endpoints []endpoint
}

type tokenResponse struct {
	Expires string `json:"expires"`
	Id      string `json:"id"` // Actual token string
	Tenant  struct {
		Id   string `json:"id"`
		Name string `json:"name"`
		// Description is a pointer since it may be null and this breaks Go < 1.1
		Description *string `json:"description"`
		Enabled     bool    `json:"enabled"`
	} `json:"tenant"`
}

type roleResponse struct {
	Id       string `json:"id"`
	Name     string `json:"name"`
	TenantId string `json:"tenantId"`
}

type userResponse struct {
	Id    string         `json:"id"`
	Name  string         `json:"name"`
	Roles []roleResponse `json:"roles"`
}

type accessWrapper struct {
	Access accessResponse `json:"access"`
}

type accessResponse struct {
	ServiceCatalog []serviceResponse `json:"serviceCatalog"`
	Token          tokenResponse     `json:"token"`
	User           userResponse      `json:"user"`
}

// keystoneAuth authenticates to OpenStack cloud using keystone v2 authentication.
//
// Uses `client` to submit HTTP requests to `URL`
// and posts `auth_data` as JSON.
func keystoneAuth(client *goosehttp.Client, auth_data interface{}, URL string) (*AuthDetails, error) {

	var accessWrapper accessWrapper
	requestData := goosehttp.RequestData{ReqValue: auth_data, RespValue: &accessWrapper}
	err := client.JsonRequest("POST", URL, "", &requestData, nil)
	if err != nil {
		return nil, err
	}

	details := &AuthDetails{}
	access := accessWrapper.Access
	respToken := access.Token
	if respToken.Id == "" {
		return nil, fmt.Errorf("authentication failed")
	}
	details.Token = respToken.Id
	details.TenantId = respToken.Tenant.Id
	details.UserId = access.User.Id
	details.RegionServiceURLs = make(map[string]ServiceURLs, len(access.ServiceCatalog))
	for _, service := range access.ServiceCatalog {
		for i, e := range service.Endpoints {
			endpointURLs, ok := details.RegionServiceURLs[e.Region]
			if !ok {
				endpointURLs = make(ServiceURLs)
				details.RegionServiceURLs[e.Region] = endpointURLs
			}
			endpointURLs[service.Type] = service.Endpoints[i].PublicURL
		}
	}
	return details, nil
}

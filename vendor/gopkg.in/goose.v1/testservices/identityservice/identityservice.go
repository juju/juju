package identityservice

import "net/http"

// An IdentityService provides user authentication for an Openstack instance.
type IdentityService interface {
	AddUser(user, secret, tenant string) *UserInfo
	FindUser(token string) (*UserInfo, error)
	RegisterServiceProvider(name, serviceType string, serviceProvider ServiceProvider)
	AddService(service Service)
	SetupHTTP(mux *http.ServeMux)
}

// Service wraps two possible Service versions
type Service struct {
	V2 V2Service
	V3 V3Service
}

// ServiceProvider is an Openstack module which has service endpoints.
type ServiceProvider interface {
	// For Keystone V2
	Endpoints() []Endpoint
	// For Keystone V3
	V3Endpoints() []V3Endpoint
}

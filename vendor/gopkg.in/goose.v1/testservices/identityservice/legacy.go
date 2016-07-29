package identityservice

import (
	"net/http"
)

type Legacy struct {
	Users
	managementURL string
}

func NewLegacy() *Legacy {
	service := &Legacy{}
	service.users = make(map[string]UserInfo)
	service.tenants = make(map[string]string)
	return service
}

func (lis *Legacy) RegisterServiceProvider(name, serviceType string, serviceProvider ServiceProvider) {
	// NOOP for legacy identity service.
}

func (lis *Legacy) AddService(service Service) {
	// NOOP for legacy identity service.
}

func (lis *Legacy) SetManagementURL(URL string) {
	lis.managementURL = URL
}

// setupHTTP attaches all the needed handlers to provide the HTTP API.
func (lis *Legacy) SetupHTTP(mux *http.ServeMux) {
	mux.Handle("/", lis)
}

func (lis *Legacy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	username := r.Header.Get("X-Auth-User")
	userInfo, ok := lis.users[username]
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	auth_key := r.Header.Get("X-Auth-Key")
	if auth_key != userInfo.secret {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if userInfo.Token == "" {
		userInfo.Token = randomHexToken()
		lis.users[username] = userInfo
	}
	header := w.Header()
	header.Set("X-Auth-Token", userInfo.Token)
	header.Set("X-Server-Management-Url", lis.managementURL+"/compute")
	header.Set("X-Storage-Url", lis.managementURL+"/object-store")
	w.WriteHeader(http.StatusNoContent)
}

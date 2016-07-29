package identityservice

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"gopkg.in/goose.v1/testservices/hook"
)

// Implement the v2 Key Pair form of identity based on Keystone

type KeyPairRequest struct {
	Auth struct {
		ApiAccessKeyCredentials struct {
			AccessKey string `json:"accessKey"`
			SecretKey string `json:"secretKey"`
		} `json:"apiAccessKeyCredentials"`
		TenantName string `json:"tenantName"`
	} `json:"auth"`
}

type KeyPair struct {
	hook.TestService
	Users
	services []V2Service
}

func NewKeyPair() *KeyPair {
	return &KeyPair{
		Users: Users{
			users:   make(map[string]UserInfo),
			tenants: make(map[string]string),
		},
	}
}

func (u *KeyPair) RegisterServiceProvider(name, serviceType string, serviceProvider ServiceProvider) {
	service := V2Service{name, serviceType, serviceProvider.Endpoints()}
	u.AddService(Service{V2: service})
}

func (u *KeyPair) AddService(service Service) {
	u.services = append(u.services, service.V2)
}

func (u *KeyPair) ReturnFailure(w http.ResponseWriter, status int, message string) {
	e := ErrorWrapper{
		Error: ErrorResponse{
			Message: message,
			Code:    status,
			Title:   http.StatusText(status),
		},
	}
	if content, err := json.Marshal(e); err != nil {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(internalError)))
		w.WriteHeader(http.StatusInternalServerError)
		w.Write(internalError)
	} else {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(content)))
		w.WriteHeader(status)
		w.Write(content)
	}
}

func (u *KeyPair) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req KeyPairRequest
	// Testing against Canonistack, all responses are application/json, even failures
	w.Header().Set("Content-Type", "application/json")
	if r.Header.Get("Content-Type") != "application/json" {
		u.ReturnFailure(w, http.StatusBadRequest, notJSON)
		return
	}
	if content, err := ioutil.ReadAll(r.Body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	} else {
		if err := json.Unmarshal(content, &req); err != nil {
			u.ReturnFailure(w, http.StatusBadRequest, notJSON)
			return
		}
	}
	userInfo, errmsg := u.authenticate(req.Auth.ApiAccessKeyCredentials.AccessKey, req.Auth.ApiAccessKeyCredentials.SecretKey)
	if errmsg != "" {
		u.ReturnFailure(w, http.StatusUnauthorized, errmsg)
		return
	}
	res, err := u.generateAccessResponse(userInfo)
	if err != nil {
		u.ReturnFailure(w, http.StatusInternalServerError, err.Error())
		return
	}
	content, err := json.Marshal(res)
	if err != nil {
		u.ReturnFailure(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write(content)
}

func (u *KeyPair) generateAccessResponse(userInfo *UserInfo) (*AccessResponse, error) {
	res := AccessResponse{}
	// We pre-populate the response with genuine entries so that it looks sane.
	if err := json.Unmarshal([]byte(exampleResponse), &res); err != nil {
		return nil, err
	}
	res.Access.ServiceCatalog = u.services
	res.Access.Token.Id = userInfo.Token
	res.Access.Token.Tenant.Id = userInfo.TenantId
	res.Access.User.Id = userInfo.Id
	if err := u.ProcessControlHook("authorisation", u, &res, userInfo); err != nil {
		return nil, err
	}
	return &res, nil
}

// setupHTTP attaches all the needed handlers to provide the HTTP API.
func (u *KeyPair) SetupHTTP(mux *http.ServeMux) {
	mux.Handle("/tokens", u)
}

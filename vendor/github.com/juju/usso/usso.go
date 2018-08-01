// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENSE file for details.

package usso

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
)

type UbuntuSSOServer struct {
	baseUrl              string
	tokenRegistrationUrl string
}

// tokenURL returns the URL where the Ubuntu SSO tokens can be requested.
func (server UbuntuSSOServer) tokenURL() string {
	return server.baseUrl + "/api/v2/tokens/oauth"
}

// AccountURL returns the URL where the Ubuntu SSO account information can be
// requested.
func (server UbuntuSSOServer) AccountsURL() string {
	return server.baseUrl + "/api/v2/accounts/"
}

// TokenDetailURL returns the URL where the Ubuntu SSO token details can be
// requested.
func (server UbuntuSSOServer) TokenDetailsURL() string {
	return server.baseUrl + "/api/v2/tokens/oauth/"
}

// LoginURL returns the url for Openid login
func (server UbuntuSSOServer) LoginURL() string {
	return server.baseUrl
}

// ProductionUbuntuSSOServer represents the production Ubuntu SSO server
// located at https://login.ubuntu.com.
var ProductionUbuntuSSOServer = UbuntuSSOServer{"https://login.ubuntu.com", "https://one.ubuntu.com/oauth/sso-finished-so-get-tokens/"}

// StagingUbuntuSSOServer represents the staging Ubuntu SSO server located
// at https://login.staging.ubuntu.com. Use it for testing.
var StagingUbuntuSSOServer = UbuntuSSOServer{"https://login.staging.ubuntu.com", "https://one.staging.ubuntu.com/oauth/sso-finished-so-get-tokens/"}

// Giving user credentials and token name, retrieves oauth credentials
// for the users, the oauth credentials can be used later to sign
// requests. If an error is returned from the identity server then it
// will be of type *Error.
func (server UbuntuSSOServer) GetToken(email string, password string, tokenName string) (*SSOData, error) {
	return server.GetTokenWithOTP(email, password, "", tokenName)
}

// GetTokenWithOTP retrieves an oauth token from the Ubuntu SSO server.
// Using the user credentials including two-factor authentication and the
// token name, an oauth token is retrieved that can later be used to sign
// requests. If an error is returned from the identity server then it
// will be of type *Error. If otp is blank then this is identical to
// GetToken.
func (server UbuntuSSOServer) GetTokenWithOTP(email, password, otp, tokenName string) (*SSOData, error) {
	credentials := map[string]string{
		"email":      email,
		"password":   password,
		"token_name": tokenName,
	}
	if otp != "" {
		credentials["otp"] = otp
	}
	jsonCredentials, err := json.Marshal(credentials)
	if err != nil {
		return nil, err
	}
	response, err := http.Post(
		server.tokenURL(),
		"application/json",
		strings.NewReader(string(jsonCredentials)))
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 && response.StatusCode != 201 {
		return nil, getError(response)
	}
	ssodata := SSOData{}
	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(body, &ssodata)
	if err != nil {
		return nil, err
	}
	ssodata.Realm = "API"
	return &ssodata, nil
}

// Error represents an error message returned from Ubuntu SSO.
type Error struct {
	Message string                 `json:"message"`
	Code    string                 `json:"code,omitempty"`
	Extra   map[string]interface{} `json:"extra,omitempty"`
}

// getError attempts to extract the most meaningful error that it can
// from a response.
func getError(resp *http.Response) *Error {
	var ssoError Error
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		ssoError.Code = resp.Status
		ssoError.Message = resp.Status
		return &ssoError
	}
	err = json.Unmarshal(body, &ssoError)
	if err != nil {
		// Attempt to pass the original error back in the best way possible
		ssoError.Code = resp.Status
		ssoError.Message = string(body)
		return &ssoError
	}
	return &ssoError
}

// Error implements error.Error.
func (err *Error) Error() string {
	if len(err.Extra) == 0 {
		return err.Message
	}
	extra := make([]string, 0, len(err.Extra))
	for k, v := range err.Extra {
		extra = append(extra, fmt.Sprintf("%s: %v", k, v))
	}
	return fmt.Sprintf("%s (%s)", err.Message, strings.Join(extra, ", "))
}

// Returns all the Ubuntu SSO information related to this account.
func (server UbuntuSSOServer) GetAccounts(ssodata *SSOData) (string, error) {
	rp := RequestParameters{
		BaseURL:         server.AccountsURL() + ssodata.ConsumerKey,
		HTTPMethod:      "GET",
		SignatureMethod: HMACSHA1{}}

	request, err := http.NewRequest(rp.HTTPMethod, rp.BaseURL, nil)
	if err != nil {
		return "", err
	}
	err = SignRequest(ssodata, &rp, request)
	if err != nil {
		return "", err
	}
	client := &http.Client{}
	response, err := client.Do(request)
	if err != nil {
		return "", err
	}

	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return "", err
	}
	if response.StatusCode == 200 {
		return string(body), nil
	} else {
		var jsonMap map[string]interface{}
		err = json.Unmarshal(body, &jsonMap)
		// In theory, this should never happen.
		if err != nil {
			return "", fmt.Errorf("NO_JSON_RESPONSE")
		}
		code, ok := jsonMap["code"]
		if !ok {
			return "", fmt.Errorf("NO_CODE")
		}
		return "", fmt.Errorf("%v", code)
	}
}

// Given oauth credentials and a request, return it signed.
func SignRequest(
	ssodata *SSOData, rp *RequestParameters, request *http.Request) error {
	return ssodata.SignRequest(rp, request)
}

// Given oauth credentials return a valid http authorization header.
func GetAuthorizationHeader(
	ssodata *SSOData, rp *RequestParameters) (string, error) {
	header, err := ssodata.GetAuthorizationHeader(rp)
	return header, err
}

// Returns all the Ubuntu SSO information related to this token.
func (server UbuntuSSOServer) GetTokenDetails(ssodata *SSOData) (string, error) {
	rp := RequestParameters{
		BaseURL:         server.TokenDetailsURL() + ssodata.TokenKey,
		HTTPMethod:      "GET",
		SignatureMethod: HMACSHA1{}}

	request, err := http.NewRequest(rp.HTTPMethod, rp.BaseURL, nil)
	if err != nil {
		return "", err
	}
	err = SignRequest(ssodata, &rp, request)
	if err != nil {
		return "", err
	}
	client := &http.Client{}
	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return "", err
	}
	if response.StatusCode == 200 {
		return string(body), nil
	} else {
		var jsonMap map[string]interface{}
		err = json.Unmarshal(body, &jsonMap)
		// due to bug #1285176, it is possible to get non json code in the response.
		if err != nil {
			return "", fmt.Errorf("INVALID_CREDENTIALS")
		}
		code, ok := jsonMap["code"]
		if !ok {
			return "", fmt.Errorf("NO_CODE")
		}
		return "", fmt.Errorf("%v", code)
	}
}

// Verify the validity of the token, abusing the API to get the token details.
func (server UbuntuSSOServer) IsTokenValid(ssodata *SSOData) (bool, error) {
	details, err := server.GetTokenDetails(ssodata)
	if details != "" && err == nil {
		return true, nil
	} else {
		return false, err
	}
}

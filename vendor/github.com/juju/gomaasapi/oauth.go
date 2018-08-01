// Copyright 2012-2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package gomaasapi

import (
	"crypto/rand"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Not a true uuidgen, but at least creates same length random
func generateNonce() (string, error) {
	randBytes := make([]byte, 16)
	_, err := rand.Read(randBytes)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%16x", randBytes), nil
}

func generateTimestamp() string {
	return strconv.Itoa(int(time.Now().Unix()))
}

type OAuthSigner interface {
	OAuthSign(request *http.Request) error
}

type OAuthToken struct {
	ConsumerKey    string
	ConsumerSecret string
	TokenKey       string
	TokenSecret    string
}

// Trick to ensure *plainTextOAuthSigner implements the OAuthSigner interface.
var _ OAuthSigner = (*plainTextOAuthSigner)(nil)

type plainTextOAuthSigner struct {
	token *OAuthToken
	realm string
}

func NewPlainTestOAuthSigner(token *OAuthToken, realm string) (OAuthSigner, error) {
	return &plainTextOAuthSigner{token, realm}, nil
}

// OAuthSignPLAINTEXT signs the provided request using the OAuth PLAINTEXT
// method: http://oauth.net/core/1.0/#anchor22.
func (signer plainTextOAuthSigner) OAuthSign(request *http.Request) error {

	signature := signer.token.ConsumerSecret + `&` + signer.token.TokenSecret
	nonce, err := generateNonce()
	if err != nil {
		return err
	}
	authData := map[string]string{
		"realm":                  signer.realm,
		"oauth_consumer_key":     signer.token.ConsumerKey,
		"oauth_token":            signer.token.TokenKey,
		"oauth_signature_method": "PLAINTEXT",
		"oauth_signature":        signature,
		"oauth_timestamp":        generateTimestamp(),
		"oauth_nonce":            nonce,
		"oauth_version":          "1.0",
	}
	// Build OAuth header.
	var authHeader []string
	for key, value := range authData {
		authHeader = append(authHeader, fmt.Sprintf(`%s="%s"`, key, url.QueryEscape(value)))
	}
	strHeader := "OAuth " + strings.Join(authHeader, ", ")
	request.Header.Add("Authorization", strHeader)
	return nil
}

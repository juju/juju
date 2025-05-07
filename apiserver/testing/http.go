// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"io"
	"net/http"

	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/testing/httptesting"

	jujuversion "github.com/juju/juju/core/version"
	jujuhttp "github.com/juju/juju/internal/http"
	"github.com/juju/juju/rpc/params"
)

// httpRequestParams holds parameters for the sendHTTPRequest methods.
type HTTPRequestParams struct {
	// do is used to make the HTTP request.
	// If it is nil, utils.GetNonValidatingHTTPClient().Do will be used.
	// If the body reader implements io.Seeker,
	// req.Body will also implement that interface.
	Do func(req *http.Request) (*http.Response, error)

	// expectError holds the error regexp to match
	// against the error returned from the HTTP Do
	// request. If it is empty, the error is expected to be
	// nil.
	ExpectError string

	// ExpectStatus holds the expected HTTP status code.
	// http.StatusOK is assumed if this is zero.
	ExpectStatus int

	// tag holds the tag to authenticate as.
	Tag string

	// password holds the password associated with the tag.
	Password string

	// method holds the HTTP method to use for the request.
	Method string

	// url holds the URL to send the HTTP request to.
	URL string

	// contentType holds the content type of the request.
	ContentType string

	// body holds the body of the request.
	Body io.Reader

	// extra headers are added to the http header
	ExtraHeaders map[string]string

	// jsonBody holds an object to be marshaled as JSON
	// as the body of the request. If this is specified, body will
	// be ignored and the Content-Type header will
	// be set to application/json.
	JSONBody interface{}

	// nonce holds the machine nonce to provide in the header.
	Nonce string
}

func SendHTTPRequest(c *tc.C, p HTTPRequestParams) *http.Response {
	c.Logf("sendRequest: %s", p.URL)
	hp := httptesting.DoRequestParams{
		Do:           p.Do,
		Method:       p.Method,
		URL:          p.URL,
		Body:         p.Body,
		JSONBody:     p.JSONBody,
		Header:       make(http.Header),
		Username:     p.Tag,
		Password:     p.Password,
		ExpectError:  p.ExpectError,
		ExpectStatus: p.ExpectStatus,
	}
	hp.Header.Set(params.JujuClientVersion, jujuversion.Current.String())
	if p.ContentType != "" {
		hp.Header.Set("Content-Type", p.ContentType)
	}
	for key, value := range p.ExtraHeaders {
		hp.Header.Set(key, value)
	}
	if p.Nonce != "" {
		hp.Header.Set(params.MachineNonceHeader, p.Nonce)
	}
	if hp.Do == nil {
		client := jujuhttp.NewClient(jujuhttp.WithSkipHostnameVerification(true))
		hp.Do = client.Do
	}
	return httptesting.Do(c, hp)
}

func AssertResponse(c *tc.C, resp *http.Response, expHTTPStatus int, expContentType string) []byte {
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(resp.StatusCode, tc.Equals, expHTTPStatus, tc.Commentf("body: %s", body))
	ctype := resp.Header.Get("Content-Type")
	c.Assert(ctype, tc.Equals, expContentType)
	return body
}

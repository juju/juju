// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"fmt"
	"net/http"

	"github.com/juju/juju/state/api/httpreq"
)

/*
TODO(ericsnow) 2014-07-01 bug #1336542
Client.AddLocalCharm() and Client.UploadTools() should
be updated to use this method (and this should be adapted to
accommodate them.  That will include adding parameters for "args" and
"payload".
*/
func (c *Client) getHTTPRequest(httpMethod string, method string) (*http.Request, error) {
	envinfo, err := c.EnvironmentInfo()
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/environment/%s/%s", c.st.serverRoot, envinfo.UUID, method)
	//	url := fmt.Sprintf("%s/%s", c.st.serverRoot, method)

	req, err := http.NewRequest(httpMethod, url, nil)
	if err != nil {
		return nil, fmt.Errorf("could not create HTTP request: %v", err)
	}

	req.SetBasicAuth(c.st.tag, c.st.password)

	return req, nil
}

func (c *Client) getRawHTTPClient() httpreq.HTTPDoer {
	return c.st.SecureHTTPClient("anything")
}

func (c *Client) sendHTTPRequest(httpMethod string, method string) (*http.Response, error) {
	req, err := c.getHTTPRequest(httpMethod, method)
	if err != nil {
		return nil, err
	}

	// Send the request.
	httpclient := c.getRawHTTPClient()
	return httpreq.Do(httpclient, req)
}

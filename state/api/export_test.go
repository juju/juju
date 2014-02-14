// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

// SetServerHostPort allows changing the URL to the internal API server.
func SetServerHostPort(c *Client, serverHostPort string) {
	c.st.serverHostPort = serverHostPort
}

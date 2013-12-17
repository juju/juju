// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

// SetAPIAddress allows changing the internal API server address that
// UploadCharm uses in order to test NotImplementedError.
func SetClientAPIAddress(c *Client, address string) {
	c.st.address = address
}

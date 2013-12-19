// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

// SetServerRoot allows changing the URL to the internal API server
// that AddLocalCharm uses in order to test NotImplementedError.
func SetServerRoot(c *Client, root string) {
	c.st.serverRoot = root
}

// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

var (
	NewWebsocketDialer = newWebsocketDialer

	WebsocketDialConfig = &websocketDialConfig
	SetUpWebsocket      = setUpWebsocket
	SlideAddressToFront = slideAddressToFront

	ValidateBackupHash = validateBackupHash
	WriteBackupFile    = writeBackupFile
)

// SetServerRoot allows changing the URL to the internal API server
// that AddLocalCharm uses in order to test NotImplementedError.
func SetServerRoot(c *Client, root string) {
	c.st.serverRoot = root
}

// SetEnvironTag patches the value of the environment tag.
// It returns a function that reverts the change.
func PatchEnvironTag(st *State, envTag string) func() {
	originalTag := st.environTag
	st.environTag = envTag
	return func() {
		st.environTag = originalTag
	}
}

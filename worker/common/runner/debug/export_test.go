// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package debug

import "io"

// FindSessionWithWriter returns the server session with the writer
// specified so the run output can be captured for tests.
func (c *HooksContext) FindSessionWithWriter(writer io.Writer) (*ServerSession, error) {
	session, err := c.FindSession()
	if session != nil {
		session.output = writer
	}
	return session, err
}

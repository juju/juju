// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package debug

// RunHookOutput returns the most recent combined output from RunHook.
func (s *ServerSession) RunHookOutput() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.output
}

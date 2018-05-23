// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

// UseTargetServer returns a new Server based on the input target node name.
// It is intended for use when operations must target specific nodes in a
// cluster.
func (s Server) UseTargetServer(name string) (*Server, error) {
	return NewServer(s.UseTarget(name))
}

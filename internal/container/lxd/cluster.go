// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import "context"

func (s *Server) ClusterSupported() bool {
	return s.clusterAPISupport
}

// UseTargetServer returns a new Server based on the input target node name.
// It is intended for use when operations must target specific nodes in a
// cluster.
func (s *Server) UseTargetServer(ctx context.Context, name string) (*Server, error) {
	logger.Debugf(ctx, "creating LXD server for cluster node %q", name)
	return NewServer(s.UseTarget(name))
}

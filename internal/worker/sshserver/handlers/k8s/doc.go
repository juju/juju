// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package k8s provides a set of handlers for SSH sessions to Kubernetes units,
// specifically to a container with a pod.
// The handlers are responsible for establishing an SSH connection to the
// target container and proxying the user's connection to the container.
package k8s

// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package pebble

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// Probe constants
const (
	alivePath = "/v1/health?level=alive"
	readyPath = "/v1/health?level=ready"
)

func StartupHandler(port string) corev1.ProbeHandler {
	return corev1.ProbeHandler{
		HTTPGet: &corev1.HTTPGetAction{
			Path: alivePath,
			Port: intstr.Parse(port),
		},
	}
}

// LivenessHandler returns a probe handler for the Kubernetes liveness probe.
// It uses the same Pebble health endpoint as StartupHandler, but they are
// separate functions because they serve different Kubernetes lifecycle checks
// and may diverge in the future.
func LivenessHandler(port string) corev1.ProbeHandler {
	return corev1.ProbeHandler{
		HTTPGet: &corev1.HTTPGetAction{
			Path: alivePath,
			Port: intstr.Parse(port),
		},
	}
}

// ReadinessHandler returns the probe handler for checking the readiness of
// the pebble-based container via a /v1/health?level=ready HTTP GET.
func ReadinessHandler(port string) corev1.ProbeHandler {
	return corev1.ProbeHandler{
		HTTPGet: &corev1.HTTPGetAction{
			Path: readyPath,
			Port: intstr.Parse(port),
		},
	}
}

// APIServerReadinessHandler returns a probe handler that checks whether the
// API server is accepting TCP connections on the given port. This is used as
// the readiness probe for the controller api-server container so that the pod
// is not added to the service endpoint until the jujuagentd has started and
// the API server is listening.
func APIServerReadinessHandler(apiPort int) corev1.ProbeHandler {
	return corev1.ProbeHandler{
		TCPSocket: &corev1.TCPSocketAction{
			Port: intstr.FromInt(apiPort),
		},
	}
}

// Reserved Pebble health check ports for each container.
// All ports from 38813 onwards are reserved for workload containers
// Additional ports should be allocated backwards
// e.g. 38812, 38811, 38810, ...
const (
	// for api-server container (jujud)
	ApiServerHealthCheckPort = "38811"

	// for charm container (containeragent)
	CharmHealthCheckPort = "38812"

	// Arbitrary, but PEBBLE -> P38813 -> Port 38813
	// workload containers start at this value and increment
	// see WorkloadHealthCheckPort function below
	workloadHealthCheckPortStart = 38813
)

// WorkloadHealthCheckPort returns the HTTP port to use for Pebble
// health checks on workload container i, where i = 0, 1, 2, ...
func WorkloadHealthCheckPort(i int) string {
	return fmt.Sprint(workloadHealthCheckPortStart + i)
}

// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package port provides domain types for managing network ports opened by
// units in Juju.
//
// Units can open ports to allow network access to their services. The port
// domain manages port ranges, protocols, and the association between units
// and their opened ports.
//
// # Key Concepts
//
// Port management includes:
//   - Port ranges: contiguous blocks of ports (e.g., 80-443)
//   - Protocols: TCP, UDP, ICMP
//   - Endpoints: named application endpoints with port mappings
//   - Unit ownership: which unit has opened specific ports
//
// # Port Lifecycle
//
// Ports are:
//   - Opened by units as they start services
//   - Managed per-unit or per-endpoint
//   - Closed when units are removed or services stop
//   - Validated to prevent conflicts within the model
package port

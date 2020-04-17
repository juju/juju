// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

// Locus represents a graph for a given set of link layer devices along with
// their associated ip addresses (if any).
//
// Extracting information about machine or provider addresses will then be
// possible.
//
//                              +---------------+
//                              |               |
//                              |               |
//                              |     LOCUS     |
//                              |               |
//                              |               |
//                              +-------+-------+
//                                      |
//                           +----------+--------+
//                           |                   |
//                   +-------+------+     +------+-------+
//                   |              |     |              |
//                   |              |     |              |
//                   |  LINK LAYER  |     |  LINK LAYER  |
//                   |    DEVICE    |     |    DEVICE    |
//                   |              |     |              |
//                   |              |     |              |
//                   +-------+------+     +--------------+
//                           |
//         +-----------------+
//         |                 |
// +-------+-------+ +-------+-------+
// |               | |               |
// |               | |               |
// |  IP ADDRESS   | |  IP ADDRESS   |
// |               | |               |
// |               | |               |
// +---------------+ +---------------+
//
type Locus struct {
	LinkLayerDevices InterfaceInfos
}

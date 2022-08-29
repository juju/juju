// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package provisioner supplies the API facade used by the provisioner worker.
//
// Versions of the API < 10 have logic dating from the original spaces
// implementation. If multiple positive space constraints are supplied,
// only the first in the list is used to determine possible subnet/AZ
// combinations suitable for machine location.
//
// Version 10+ will consider all supplied positive space constraints when
// making this determination.
package provisioner

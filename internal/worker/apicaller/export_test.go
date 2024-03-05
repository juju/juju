// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apicaller

// Strategy is a wart left over from the original implementation;
// ideally we'd be using a clock and configuring this approach
// explicitly, but (again, as usual) can't fix everything at once.
var Strategy = &checkProvisionedStrategy

// NewConnFacade is a dirty hack; should be explicit config; not
// currently convenient.
var NewConnFacade = &newConnFacade

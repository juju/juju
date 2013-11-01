// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"fmt"
	"time"

	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/utils"
)

// Use ShortAttempt to poll for short-term events.
// TODO: This may need tuning for different providers (or even environments).
var ShortAttempt = utils.AttemptStrategy{
	Total: 5 * time.Second,
	Delay: 200 * time.Millisecond,
}

// A request may fail to due "eventual consistency" semantics, which
// should resolve fairly quickly.  These delays are specific to the provider
// and best tuned there.
// Other requests fail due to a slow state transition (e.g. an instance taking
// a while to release a security group after termination).  If you need to
// poll for the latter kind, use LongAttempt.
var LongAttempt = utils.AttemptStrategy{
	Total: 3 * time.Minute,
	Delay: 1 * time.Second,
}

// WaitDNSName is an implementation that the providers can use.  It builds on
// the provider's implementation of Instance.DNSName.
func WaitDNSName(inst instance.Instance) (string, error) {
	for a := LongAttempt.Start(); a.Next(); {
		name, err := inst.DNSName()
		if err == nil || err != instance.ErrNoDNSName {
			return name, err
		}
	}
	return "", fmt.Errorf("timed out trying to get DNS address for %v", inst.Id())
}

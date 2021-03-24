// Copyright 2013 Joyent Inc.
// Licensed under the AGPLv3, see LICENCE file for details.

package joyent

import (
	"time"

	"github.com/juju/utils/v2"
)

// Use ShortAttempt to poll for short-term events.
//
// TODO(katco): 2016-08-09: lp:1611427
var (
	ShortAttempt = utils.AttemptStrategy{
		Total: 5 * time.Second,
		Delay: 200 * time.Millisecond,
	}

	Provider              = GetProviderInstance()
	GetPorts              = getRules
	CreateFirewallRuleAll = createFirewallRuleAll
	CreateFirewallRuleVm  = createFirewallRuleVm
)

// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import (
	"github.com/juju/errors"

	"github.com/juju/juju/internal/proxy"
)

// Proxy represents a proxy connection info.
type Proxy struct {
	Config map[string]interface{} `json:"config"`
	Type   string                 `json:"type"`
}

// NewProxy constructs a new Proxy from the supplied proxier.
func NewProxy(proxier proxy.Proxier) (*Proxy, error) {
	if proxier == nil {
		return nil, errors.NotValidf("cannot have nil proxier")
	}

	config, err := proxier.RawConfig()
	if err != nil {
		return nil, errors.Annotatef(err, "getting raw configuration for proxier of type %s", proxier.Type())
	}

	return &Proxy{
		Config: config, Type: proxier.Type(),
	}, nil
}

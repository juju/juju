// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
)

var (
	logger            = loggo.GetLogger("juju.provider.gce")
	errNotImplemented = errors.NotImplementedf("in gce provider")
)

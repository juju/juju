// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package logfwd contains the tools needed to do log record
// forwarding in Juju. The common code sits at the top level. The
// different forwarding targets (e.g. syslog) are provided through
// sub-packages.
package logfwd

// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"net/http"

	"github.com/juju/errors"

	"github.com/juju/juju/internal/version"
	"github.com/juju/juju/rpc/params"
)

// JujuClientVersionFromRequest returns the Juju client version
// number from the HTTP request.
func JujuClientVersionFromRequest(req *http.Request) (version.Number, error) {
	verStr := req.Header.Get(params.JujuClientVersion)
	if verStr == "" {
		return version.Zero, errors.New(`missing "X-Juju-ClientVersion" in request headers`)
	}
	ver, err := version.Parse(verStr)
	if err != nil {
		return version.Zero, errors.Annotatef(err, "invalid X-Juju-ClientVersion %q", verStr)
	}
	return ver, nil
}

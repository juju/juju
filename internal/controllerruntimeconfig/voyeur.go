// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controllerruntimeconfig

import "github.com/juju/utils/v4/voyeur"

// NewRuntimeConfigChanged returns a new voyeur.Value that is used to
// notify workers when the controller runtime configuration has changed.
// Workers that depend on controller-local startup values (such as the
// controller-log-router) watch this value and re-read the current config
// when it is set. The value is set by the controllerlokiupdater worker
// after it has written updated Loki endpoint settings to runtime.conf.
func NewRuntimeConfigChanged() *voyeur.Value {
	return voyeur.NewValue(false)
}

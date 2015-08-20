// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

func SetUniterObserver(u *Uniter, observer UniterExecutionObserver) {
	u.observer = observer
}

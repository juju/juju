// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

func SetUniterObserver(u *Uniter, observer UniterExecutionObserver) {
	u.observer = observer
}

func RunCommands(u *Uniter, commands string) (results *RunResults, err error) {
	return u.runCommands(commands)
}

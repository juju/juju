// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"

	"github.com/juju/juju/core/leadership"
)

type noopEnsurer struct{}

// NoopLeaderEnsurer returns a leadership.Ensurer that will always allow
// leadership to be taken and held.
func NoopLeaderEnsurer() leadership.Ensurer {
	return noopEnsurer{}
}

// LeadershipCheck returns a Token representing the supplied unit's
// application leadership. The existence of the token does not imply
// its accuracy; you need to Check() it.
//
// This method returns a token that accepts a *[]txn.Op, into which
// it will (on success) copy mgo/txn operations that can be used to
// verify the unit's continued leadership as part of another txn.
func (noopEnsurer) LeadershipCheck(applicationId, unitId string) leadership.Token {
	return noopToken{}
}

// WithLeader ensures that the input unit holds leadership of the input
// application for the duration of execution of the input function.
func (noopEnsurer) WithLeader(ctx context.Context, appName, unitName string, fn func(context.Context) error) error {
	return fn(ctx)
}

type noopToken struct{}

// Check returns whether the leadership token is still valid.
func (noopToken) Check() error {
	return nil
}

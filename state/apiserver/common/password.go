// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
)

type PasswordChanger struct {
	st        authGetter
	canChange func(tag string) bool
}

type authGetter interface {
	Authenticator(tag string) (state.TaggedAuthenticator, error)
}

func NewPasswordChanger(st *state.State, canChange func(tag string) bool) *PasswordChanger {
	return &PasswordChanger{
		st:        st,
		canChange: canChange,
	}
}

func (pc *PasswordChanger) SetPasswords(args params.PasswordChanges) params.ErrorResults {
	results := params.ErrorResults{
		Errors: make([]*params.Error, len(args.Changes)),
	}
	for i, param := range args.Changes {
		if !pc.canChange(param.Tag) {
			results.Errors[i] = ServerError(ErrPerm)
			continue
		}
		if err := pc.setPassword(param.Tag, param.Password); err != nil {
			results.Errors[i] = ServerError(err)
		}
	}
	return results
}

func (pc *PasswordChanger) setPassword(tag, password string) error {
	type mongoPassworder interface {
		SetMongoPassword(password string) error
	}
	entity, err := pc.st.Authenticator(tag)
	if err != nil {
		return err
	}
	// We set the mongo password first on the grounds that
	// if it fails, the agent in question should still be able
	// to authenticate to another API server and ask it to change
	// its password.
	if entity, ok := entity.(mongoPassworder); ok {
		// TODO(rog) when the API is universal, check that the entity is a
		// machine with jobs that imply it needs access to the mongo state.
		if err := entity.SetMongoPassword(password); err != nil {
			return err
		}
	}
	return entity.SetPassword(password)
}

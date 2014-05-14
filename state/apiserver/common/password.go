// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/loggo"

	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
)

var logger = loggo.GetLogger("juju.state.apiserver.common")

// PasswordChanger implements a common SetPasswords method for use by
// various facades.
type PasswordChanger struct {
	st           state.EntityFinder
	getCanChange GetAuthFunc
}

// NewPasswordChanger returns a new PasswordChanger. The GetAuthFunc will be
// used on each invocation of SetPasswords to determine current permissions.
func NewPasswordChanger(st state.EntityFinder, getCanChange GetAuthFunc) *PasswordChanger {
	return &PasswordChanger{
		st:           st,
		getCanChange: getCanChange,
	}
}

// SetPasswords sets the given password for each supplied entity, if possible.
func (pc *PasswordChanger) SetPasswords(args params.EntityPasswords) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Changes)),
	}
	if len(args.Changes) == 0 {
		return result, nil
	}
	canChange, err := pc.getCanChange()
	if err != nil {
		return params.ErrorResults{}, err
	}
	for i, param := range args.Changes {
		if !canChange(param.Tag) {
			result.Results[i].Error = ServerError(ErrPerm)
			continue
		}
		if err := pc.setPassword(param.Tag, param.Password); err != nil {
			result.Results[i].Error = ServerError(err)
		}
	}
	return result, nil
}

func (pc *PasswordChanger) setMongoPassword(entity state.Entity, password string) error {
	type mongoPassworder interface {
		SetMongoPassword(password string) error
	}
	// We set the mongo password first on the grounds that
	// if it fails, the agent in question should still be able
	// to authenticate to another API server and ask it to change
	// its password.
	if entity0, ok := entity.(mongoPassworder); ok {
		if err := entity0.SetMongoPassword(password); err != nil {
			return err
		}
		logger.Infof("setting mongo password for %q", entity.Tag())
		return nil
	}
	return NotSupportedError(entity.Tag(), "mongo access")
}

func (pc *PasswordChanger) setPassword(tag, password string) error {
	type jobsGetter interface {
		Jobs() []state.MachineJob
	}
	var err error
	entity0, err := pc.st.FindEntity(tag)
	if err != nil {
		return err
	}
	entity, ok := entity0.(state.Authenticator)
	if !ok {
		return NotSupportedError(tag, "authentication")
	}
	if entity, ok := entity0.(jobsGetter); ok {
		for _, job := range entity.Jobs() {
			paramsJob := job.ToParams()
			if paramsJob.NeedsState() {
				err = pc.setMongoPassword(entity0, password)
				break
			}
		}
	}
	if err == nil {
		err = entity.SetPassword(password)
		logger.Infof("setting password for %q", tag)
	}
	return err
}

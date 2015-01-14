// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"fmt"

	//"github.com/juju/errors"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/names"

	"github.com/juju/juju/state"
)

type AgentStatusSetter struct {
	st           state.EntityFinder
	getCanModify GetAuthFunc
}

// NewAgentStatusSetter returns a new AgentStatusSetter. The GetAuthFunc will be
// used on each invocation of SetStatus to determine current
// permissions.
func NewAgentStatusSetter(st state.EntityFinder, getCanModify GetAuthFunc) *AgentStatusSetter {
	return &AgentStatusSetter{
		st:           st,
		getCanModify: getCanModify,
	}
}

func (s *AgentStatusSetter) setAgentEntityStatus(tag names.Tag, status params.Status, info string, data map[string]interface{}) error {
	entity, err := s.st.FindEntity(tag)
	if err != nil {
		return err
	}
	switch entity := entity.(type) {
	case state.AgentUnit:
		agent := entity.Agent()
		return agent.SetStatus(state.Status(status), info, data)
	default:
		return NotSupportedError(tag, fmt.Sprintf("setting agent status, %T", entity))
	}
}

// SetStatus sets the status of each given entity.
func (s *AgentStatusSetter) SetAgentStatus(args params.SetStatus) (params.ErrorResults, error) {
	result := params.ErrorResults{
		//TODO: This should be a param just for this, as should be params.SetStatus
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	if len(args.Entities) == 0 {
		return result, nil
	}
	canModify, err := s.getCanModify()
	if err != nil {
		return params.ErrorResults{}, err
	}
	for i, arg := range args.Entities {
		tag, err := names.ParseTag(arg.Tag)
		if err != nil {
			result.Results[i].Error = ServerError(ErrPerm)
			continue
		}
		err = ErrPerm
		if canModify(tag) {
			err = s.setAgentEntityStatus(tag, arg.Status, arg.Info, arg.Data)
		}
		result.Results[i].Error = ServerError(err)
	}
	return result, nil
}

package common

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

// RebootRequester implements the RequestReboot API method
type RebootRequester struct {
	st   state.EntityFinder
	auth GetAuthFunc
}

func NewRebootRequester(st state.EntityFinder, auth GetAuthFunc) *RebootRequester {
	return &RebootRequester{
		st:   st,
		auth: auth,
	}
}

func (r *RebootRequester) oneRequest(tag names.Tag) error {
	entity0, err := r.st.FindEntity(tag)
	if err != nil {
		return err
	}
	entity, ok := entity0.(state.RebootFlagSetter)
	if !ok {
		return NotSupportedError(tag, "request reboot")
	}
	return entity.SetRebootFlag(true)
}

// RequestReboot sets the reboot flag on the provided machines
func (r *RebootRequester) RequestReboot(args params.Entities) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	if len(args.Entities) == 0 {
		return result, nil
	}
	auth, err := r.auth()
	if err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = ServerError(ErrPerm)
			continue
		}
		err = ErrPerm
		if auth(tag) {
			err = r.oneRequest(tag)
		}
		result.Results[i].Error = ServerError(err)
	}
	return result, nil
}

// RebootActionGetter implements the GetRebootAction API method
type RebootActionGetter struct {
	st   state.EntityFinder
	auth GetAuthFunc
}

func NewRebootActionGetter(st state.EntityFinder, auth GetAuthFunc) *RebootActionGetter {
	return &RebootActionGetter{
		st:   st,
		auth: auth,
	}
}

func (r *RebootActionGetter) getOneAction(tag names.Tag) (params.RebootAction, error) {
	entity0, err := r.st.FindEntity(tag)
	if err != nil {
		return "", err
	}
	entity, ok := entity0.(state.RebootActionGetter)
	if !ok {
		return "", NotSupportedError(tag, "request reboot")
	}
	rAction, err := entity.ShouldRebootOrShutdown()
	if err != nil {
		return params.ShouldDoNothing, err
	}
	return params.RebootAction(rAction), nil
}

// GetRebootAction returns the action a machine agent should take.
// If a reboot flag is set on the machine, then that machine is
// expected to reboot (params.ShouldReboot).
// a reboot flag set on the machine parent or grandparent, will
// cause the machine to shutdown (params.ShouldShutdown).
// If no reboot flag is set, the machine should do nothing (params.ShouldDoNothing).
func (r *RebootActionGetter) GetRebootAction(args params.Entities) (params.RebootActionResults, error) {
	result := params.RebootActionResults{
		Results: make([]params.RebootActionResult, len(args.Entities)),
	}
	if len(args.Entities) == 0 {
		return result, nil
	}
	auth, err := r.auth()
	if err != nil {
		return params.RebootActionResults{}, errors.Trace(err)
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = ServerError(ErrPerm)
			continue
		}
		err = ErrPerm
		if auth(tag) {
			result.Results[i].Result, err = r.getOneAction(tag)
		}
		result.Results[i].Error = ServerError(err)
	}
	return result, nil
}

// RebootFlagClearer implements the ClearReboot API call
type RebootFlagClearer struct {
	st   state.EntityFinder
	auth GetAuthFunc
}

func NewRebootFlagClearer(st state.EntityFinder, auth GetAuthFunc) *RebootFlagClearer {
	return &RebootFlagClearer{
		st:   st,
		auth: auth,
	}
}

func (r *RebootFlagClearer) clearOneFlag(tag names.Tag) error {
	entity0, err := r.st.FindEntity(tag)
	if err != nil {
		return err
	}
	entity, ok := entity0.(state.RebootFlagSetter)
	if !ok {
		return NotSupportedError(tag, "clear reboot flag")
	}
	return entity.SetRebootFlag(false)
}

// ClearReboot will clear the reboot flag on provided machines, if it exists.
func (r *RebootFlagClearer) ClearReboot(args params.Entities) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	if len(args.Entities) == 0 {
		return result, nil
	}
	auth, err := r.auth()
	if err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = ServerError(ErrPerm)
			continue
		}
		err = ErrPerm
		if auth(tag) {
			err = r.clearOneFlag(tag)
		}
		result.Results[i].Error = ServerError(err)
	}
	return result, nil
}

// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

type Action struct {
	name   string
	params map[string]interface{}
}

func NewAction(name string, params map[string]interface{}) (*Action, error) {
	newParams := make(map[string]interface{})
	for key, param := range params {
		newParams[key] = param
	}
	return &Action{name: name, params: newParams}, nil
}

func (a *Action) Name() string {
	return a.name
}

func (a *Action) Params() map[string]interface{} {
	return a.params
}

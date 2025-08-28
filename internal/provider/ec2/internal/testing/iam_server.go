// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"sync"

	"github.com/aws/aws-sdk-go-v2/service/iam/types"
)

type InlinePolicy struct {
	PolicyDocument *string
	PolicyName     *string
}

// IAMServer implements an IAM simulator for use in testing
type IAMServer struct {
	mu sync.Mutex

	instanceProfiles       map[string]*types.InstanceProfile
	roles                  map[string]*types.Role
	roleInlinePolicy       map[string]*InlinePolicy
	producePermissionError bool
}

func NewIAMServer() (*IAMServer, error) {
	srv := &IAMServer{}
	srv.Reset()
	return srv, nil
}

func (i *IAMServer) ProducePermissionError(p bool) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.producePermissionError = p
}

func (i *IAMServer) Reset() {
	i.mu.Lock()
	defer i.mu.Unlock()

	i.instanceProfiles = make(map[string]*types.InstanceProfile)
	i.roles = make(map[string]*types.Role)
	i.roleInlinePolicy = make(map[string]*InlinePolicy)
}

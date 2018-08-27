// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/cloud"
)

// InvalidateModelCredential invalidate cloud credential for the model
// of the given state.
func (st *State) InvalidateModelCredential(reason string) error {
	m, err := st.Model()
	if err != nil {
		return errors.Trace(err)
	}

	tag, exists := m.CloudCredential()
	if !exists {
		// Model is on the cloud that does not require auth - nothing to do.
		return nil
	}

	return errors.Trace(st.InvalidateCloudCredential(tag, reason))
}

// ValidateCloudCredential validates new cloud credential for this model.
func (m *Model) ValidateCloudCredential(tag names.CloudCredentialTag, credential cloud.Credential) error {
	cloud, err := m.st.Cloud(m.Cloud())
	if err != nil {
		return errors.Annotatef(err, "getting cloud %q", m.Cloud())
	}

	err = validateCredentialForCloud(cloud, tag, convertCloudCredentialToState(tag, credential))
	if err != nil {
		return errors.Annotatef(err, "validating credential %q for cloud %q", tag.Id(), cloud.Name)
	}
	return nil
}

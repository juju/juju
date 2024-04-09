// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"

	dbcloud "github.com/juju/juju/domain/cloud/state"
	"github.com/juju/juju/domain/credential"
)

// These structs represent the persistent cloud credential entity schema in the database.

type Credential struct {
	// ID holds the cloud credential document key.
	ID string `db:"uuid"`

	// CloudUUID holds the cloud reference.
	CloudUUID string `db:"cloud_uuid"`

	// AuthTypeID holds the auth type reference.
	AuthTypeID int `db:"auth_type_id"`

	// Name is the name of the credential.
	Name string `db:"name"`

	// Owner is the user who owns the credential.
	// TODO(wallyworld) - this will be a user reference when users are added.
	OwnerUUID string `db:"owner_uuid"`

	// Revoked is true if the credential has been revoked.
	Revoked bool `db:"revoked"`

	// Invalid stores flag that indicates if a credential is invalid.
	// Note that the credential is valid:
	//  * if the flag is explicitly set to 'false'; or
	//  * if the flag is not set at all, as will be the case for
	//    new inserts or credentials created with previous Juju versions. In
	//    this case, we'd still read it as 'false' and the credential validity
	//    will be interpreted correctly.
	// This flag will need to be explicitly set to 'true' for a credential
	// to be considered invalid.
	Invalid bool `db:"invalid"`

	// InvalidReason contains the reason why the credential was marked as invalid.
	// This can range from cloud messages such as an expired credential to
	// commercial reasons set via CLI or api calls.
	InvalidReason string `db:"invalid_reason"`
}

// CredentialAttribute represents the persistent credential attributes schema
// in the database.
type CredentialAttribute struct {
	// CredentialUUID holds the parent cloud credential document key.
	CredentialUUID string `db:"cloud_credential_uuid"`

	// Key is the attribute key.
	Key string `db:"key"`

	// Value is the attribute value.
	Value string `db:"value"`
}

type Credentials []Credential

// ToCloudCredentials converts the given credentials to a slice of cloud credentials.
func (rows Credentials) ToCloudCredentials(authTypes []dbcloud.AuthType, clouds []dbcloud.Cloud, keyValues []CredentialAttribute) ([]credential.CloudCredentialResult, error) {
	if n := len(rows); n != len(authTypes) || n != len(keyValues) || n != len(clouds) {
		// Should never happen.
		return nil, errors.New("row length mismatch")
	}

	var result []credential.CloudCredentialResult
	recordResult := func(row *Credential, authType, cloudName string, attrs credentialAttrs) {
		result = append(result, credential.CloudCredentialResult{
			CloudCredentialInfo: credential.CloudCredentialInfo{
				AuthType:      authType,
				Attributes:    attrs,
				Revoked:       row.Revoked,
				Label:         row.Name,
				Invalid:       row.Invalid,
				InvalidReason: row.InvalidReason,
			},
			CloudName: cloudName,
		})
	}

	var (
		current             *Credential
		authType, cloudName string
		attrs               = make(credentialAttrs)
	)
	for i, row := range rows {
		if current != nil && row.ID != current.ID {
			recordResult(current, authType, cloudName, attrs)
			attrs = make(credentialAttrs)
		}
		authType = authTypes[i].Type
		cloudName = clouds[i].Name
		if keyValues[i].Key != "" {
			attrs[keyValues[i].Key] = keyValues[i].Value
		}
		rowCopy := row
		current = &rowCopy
	}
	if current != nil {
		recordResult(current, authType, cloudName, attrs)
	}
	return result, nil
}

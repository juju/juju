// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controlsocket

import (
	"context"
	stderrors "errors"
	"fmt"
	"testing"

	jujuerrors "github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	coreobjectstore "github.com/juju/juju/core/objectstore"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type preflightValidatorSuite struct{}

func TestPreflightValidatorSuite(t *testing.T) {
	tc.Run(t, &preflightValidatorSuite{})
}

func (s *preflightValidatorSuite) TestConfigValidateSuccess(c *tc.C) {
	config := s.validConfig(c)
	c.Check(config.Validate(), tc.ErrorIsNil)
}

func (s *preflightValidatorSuite) TestConfigValidateNilControllerService(c *tc.C) {
	config := s.validConfig(c)
	config.ControllerService = nil
	c.Check(config.Validate(), tc.ErrorMatches, ".*nil ControllerService.*")
}

func (s *preflightValidatorSuite) TestValidateNoMissingFiles(c *tc.C) {
	config := s.validConfig(c)
	config.ControllerMetadataService = stubMetadataService{
		metadata: []coreobjectstore.Metadata{{
			Path:   "controller-tools",
			SHA384: "hash-controller",
		}},
	}
	config.ControllerService = stubControllerService{
		namespaces: []string{"model-1"},
	}
	config.ObjectStoreServicesGetter = stubObjectStoreServicesGetter{
		byModel: map[model.UUID]MetadataService{
			"model-1": stubMetadataService{
				metadata: []coreobjectstore.Metadata{{
					Path:   "model-object",
					SHA384: "hash-model",
				}},
			},
		},
	}
	config.NewHashFileSystemAccessor = makeNamespaceHashAccessor(
		map[string]map[string]error{},
	)

	validator, err := NewDrainPreflightValidator(config)
	c.Assert(err, tc.ErrorIsNil)

	missing, err := validator.Validate(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(missing, tc.HasLen, 0)
}

func (s *preflightValidatorSuite) TestValidateMissingFiles(c *tc.C) {
	config := s.validConfig(c)
	config.ControllerMetadataService = stubMetadataService{
		metadata: []coreobjectstore.Metadata{{
			Path:   "controller-tools",
			SHA384: "hash-controller",
		}},
	}
	config.ControllerService = stubControllerService{
		namespaces: []string{"model-1"},
	}
	config.ObjectStoreServicesGetter = stubObjectStoreServicesGetter{
		byModel: map[model.UUID]MetadataService{
			"model-1": stubMetadataService{
				metadata: []coreobjectstore.Metadata{{
					Path:   "model-object",
					SHA384: "hash-model",
				}},
			},
		},
	}
	config.NewHashFileSystemAccessor = makeNamespaceHashAccessor(
		map[string]map[string]error{
			"controller": {
				"hash-controller": jujuerrors.NotFoundf("missing"),
			},
			"model-1": {
				"hash-model": jujuerrors.NotFoundf("missing"),
			},
		},
	)

	validator, err := NewDrainPreflightValidator(config)
	c.Assert(err, tc.ErrorIsNil)

	missing, err := validator.Validate(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(missing, tc.DeepEquals, []MissingObject{
		{
			Namespace: "controller",
			Path:      "controller-tools",
			Hash:      "hash-controller",
		},
		{
			Namespace: "model-1",
			Path:      "model-object",
			Hash:      "hash-model",
		},
	})
}

func (s *preflightValidatorSuite) TestValidateListMetadataError(c *tc.C) {
	config := s.validConfig(c)
	config.ControllerService = stubControllerService{
		namespaces: []string{"model-1"},
	}
	config.ObjectStoreServicesGetter = stubObjectStoreServicesGetter{
		byModel: map[model.UUID]MetadataService{
			"model-1": stubMetadataService{
				err: stderrors.New("boom"),
			},
		},
	}

	validator, err := NewDrainPreflightValidator(config)
	c.Assert(err, tc.ErrorIsNil)

	_, err = validator.Validate(c.Context())
	c.Assert(err, tc.ErrorMatches, `.*validating object store files for model namespace "model-1".*boom.*`)
}

func (s *preflightValidatorSuite) TestMissingObjectsErrorIncludesReadRepair(c *tc.C) {
	err := missingObjectsError([]MissingObject{{
		Namespace: "controller",
		Path:      "tools/juju",
		Hash:      "abc",
	}})
	c.Assert(err, tc.ErrorMatches, ".*drain is not viable.*read-repair.*")
}

func (s *preflightValidatorSuite) TestMissingObjectsErrorCapsOutput(c *tc.C) {
	missing := make([]MissingObject, 0, maxMissingObjectsInError+1)
	for i := range maxMissingObjectsInError + 1 {
		missing = append(missing, MissingObject{
			Namespace: "controller",
			Path:      fmt.Sprintf("path-%03d", i),
			Hash:      fmt.Sprintf("hash-%03d", i),
		})
	}

	err := missingObjectsError(missing)
	c.Assert(err, tc.ErrorMatches, ".*showing first 100.*")
}

func (s *preflightValidatorSuite) validConfig(c *tc.C) DrainPreflightValidatorConfig {
	return DrainPreflightValidatorConfig{
		ControllerService: stubControllerService{},
		ControllerMetadataService: stubMetadataService{
			metadata: nil,
		},
		ObjectStoreServicesGetter: stubObjectStoreServicesGetter{
			byModel: map[model.UUID]MetadataService{},
		},
		NewHashFileSystemAccessor: makeNamespaceHashAccessor(
			map[string]map[string]error{},
		),
		SelectFileHash: func(m coreobjectstore.Metadata) string {
			return m.SHA384
		},
		RootDir: c.MkDir(),
		Logger:  loggertesting.WrapCheckLog(c),
	}
}

type stubControllerService struct {
	namespaces []string
	err        error
}

func (s stubControllerService) GetModelNamespaces(context.Context) ([]string, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.namespaces, nil
}

type stubMetadataService struct {
	metadata []coreobjectstore.Metadata
	err      error
}

func (s stubMetadataService) ListMetadata(context.Context) ([]coreobjectstore.Metadata, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.metadata, nil
}

type stubObjectStoreServicesGetter struct {
	byModel map[model.UUID]MetadataService
}

func (s stubObjectStoreServicesGetter) ObjectStoreForModel(modelUUID model.UUID) MetadataService {
	return s.byModel[modelUUID]
}

type stubHashFileSystemAccessor struct {
	errorsByHash map[string]error
}

func (s stubHashFileSystemAccessor) HashExists(_ context.Context, hash string) error {
	if err, ok := s.errorsByHash[hash]; ok {
		return err
	}
	return nil
}

func makeNamespaceHashAccessor(
	errorsByNamespace map[string]map[string]error,
) NewHashFileSystemAccessorFunc {
	return func(namespace string, _ string, _ logger.Logger) HashFileSystemAccessor {
		return stubHashFileSystemAccessor{
			errorsByHash: errorsByNamespace[namespace],
		}
	}
}

// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controlsocket

import (
	"context"
	"fmt"
	"slices"
	"strings"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	coreobjectstore "github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/internal/errors"
)

const (
	controllerNamespace      = "controller"
	maxMissingObjectsInError = 100
)

// DrainPreflightValidator validates that a drain can start on the current
// primary controller.
type DrainPreflightValidator interface {
	// Validate returns all missing objects that would block a safe drain.
	Validate(ctx context.Context) ([]MissingObject, error)
}

// MissingObject identifies an object that is referenced in metadata but not
// present on local disk for the corresponding namespace.
type MissingObject struct {
	Namespace string
	Path      string
	Hash      string
}

// ControllerService provides access to controller model namespaces.
type ControllerService interface {
	// GetModelNamespaces returns the model namespaces of all models in state.
	GetModelNamespaces(ctx context.Context) ([]string, error)
}

// MetadataService provides object store metadata operations needed by preflight.
type MetadataService interface {
	// ListMetadata returns object metadata for the namespace.
	ListMetadata(ctx context.Context) ([]coreobjectstore.Metadata, error)
}

// ObjectStoreServicesGetter provides model-scoped metadata services.
type ObjectStoreServicesGetter interface {
	// ObjectStoreForModel returns the metadata service for the supplied model.
	ObjectStoreForModel(modelUUID model.UUID) MetadataService
}

// HashFileSystemAccessor checks for hash file presence on local disk.
type HashFileSystemAccessor interface {
	// HashExists returns nil if hash exists, or NotFound if it does not.
	HashExists(ctx context.Context, hash string) error
}

// NewHashFileSystemAccessorFunc creates a hash accessor for namespace/rootDir.
type NewHashFileSystemAccessorFunc func(
	namespace, rootDir string, logger logger.Logger,
) HashFileSystemAccessor

// SelectFileHashFunc selects the hash used by the file-backed object store.
type SelectFileHashFunc func(coreobjectstore.Metadata) string

// DrainPreflightValidatorConfig contains dependencies for viability checks.
type DrainPreflightValidatorConfig struct {
	ControllerService         ControllerService
	ControllerMetadataService MetadataService
	ObjectStoreServicesGetter ObjectStoreServicesGetter
	NewHashFileSystemAccessor NewHashFileSystemAccessorFunc
	SelectFileHash            SelectFileHashFunc
	RootDir                   string
	Logger                    logger.Logger
}

// Validate returns an error if config cannot drive a preflight validator.
func (config DrainPreflightValidatorConfig) Validate() error {
	if config.ControllerService == nil {
		return errors.New("nil ControllerService").Add(coreerrors.NotValid)
	}
	if config.ControllerMetadataService == nil {
		return errors.New("nil ControllerMetadataService").Add(coreerrors.NotValid)
	}
	if config.ObjectStoreServicesGetter == nil {
		return errors.New("nil ObjectStoreServicesGetter").Add(coreerrors.NotValid)
	}
	if config.NewHashFileSystemAccessor == nil {
		return errors.New("nil NewHashFileSystemAccessor").Add(coreerrors.NotValid)
	}
	if config.SelectFileHash == nil {
		return errors.New("nil SelectFileHash").Add(coreerrors.NotValid)
	}
	if config.RootDir == "" {
		return errors.New("empty RootDir").Add(coreerrors.NotValid)
	}
	if config.Logger == nil {
		return errors.New("nil Logger").Add(coreerrors.NotValid)
	}
	return nil
}

type drainPreflightValidator struct {
	controllerService         ControllerService
	controllerMetadataService MetadataService
	objectStoreServicesGetter ObjectStoreServicesGetter
	newHashFileSystemAccessor NewHashFileSystemAccessorFunc
	selectFileHash            SelectFileHashFunc
	rootDir                   string
	logger                    logger.Logger
}

// NewDrainPreflightValidator creates a validator for objectstore drain
// viability on the primary controller.
func NewDrainPreflightValidator(config DrainPreflightValidatorConfig) (DrainPreflightValidator, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Capture(err)
	}

	return &drainPreflightValidator{
		controllerService:         config.ControllerService,
		controllerMetadataService: config.ControllerMetadataService,
		objectStoreServicesGetter: config.ObjectStoreServicesGetter,
		newHashFileSystemAccessor: config.NewHashFileSystemAccessor,
		selectFileHash:            config.SelectFileHash,
		rootDir:                   config.RootDir,
		logger:                    config.Logger,
	}, nil
}

// Validate checks controller and model namespaces for metadata entries whose
// backing files are missing on the local primary controller disk.
func (v *drainPreflightValidator) Validate(ctx context.Context) ([]MissingObject, error) {
	// Validate the controller namespace first. If controller-local blobs are
	// missing, draining is unsafe regardless of model state.
	controllerMissing, err := v.validateNamespace(
		ctx,
		controllerNamespace,
		v.controllerMetadataService,
	)
	if err != nil {
		return nil, errors.Errorf("validating controller object store files: %w", err)
	}

	namespaces, err := v.controllerService.GetModelNamespaces(ctx)
	if err != nil {
		return nil, errors.Errorf("getting model namespaces: %w", err)
	}

	// Aggregate all missing objects across controller and model namespaces so
	// callers can return a complete operator-facing error in one response.
	missing := append([]MissingObject(nil), controllerMissing...)
	for _, namespace := range uniqueNamespaces(namespaces) {
		metadataService := v.objectStoreServicesGetter.ObjectStoreForModel(
			model.UUID(namespace),
		)
		if metadataService == nil {
			return nil, errors.Errorf(
				"no object store metadata service for model namespace %q",
				namespace,
			)
		}

		namespaceMissing, err := v.validateNamespace(
			ctx,
			namespace,
			metadataService,
		)
		if err != nil {
			return nil, errors.Errorf(
				"validating object store files for model namespace %q: %w",
				namespace,
				err,
			)
		}

		missing = append(missing, namespaceMissing...)
	}

	return missing, nil
}

func (v *drainPreflightValidator) validateNamespace(
	ctx context.Context,
	namespace string,
	metadataService MetadataService,
) ([]MissingObject, error) {
	metadata, err := metadataService.ListMetadata(ctx)
	if err != nil {
		return nil, errors.Errorf("listing metadata: %w", err)
	}

	if len(metadata) == 0 {
		return nil, nil
	}

	fileSystem := v.newHashFileSystemAccessor(namespace, v.rootDir, v.logger)
	if fileSystem == nil {
		return nil, errors.Errorf("creating hash file system accessor")
	}

	missing := make([]MissingObject, 0)
	for _, entry := range metadata {
		hash := v.selectFileHash(entry)
		if hash == "" {
			return nil, errors.Errorf(
				"empty file hash for namespace %q path %q",
				namespace,
				entry.Path,
			)
		}

		// A missing local hash means this primary cannot safely perform a full
		// drain. We collect it and let the caller fail with a full report.
		if err := fileSystem.HashExists(ctx, hash); errors.Is(err, coreerrors.NotFound) {
			missing = append(missing, MissingObject{
				Namespace: namespace,
				Path:      entry.Path,
				Hash:      hash,
			})
		} else if err != nil {
			return nil, errors.Errorf(
				"checking file hash %q for path %q: %w",
				hash,
				entry.Path,
				err,
			)
		}
	}

	return missing, nil
}

func missingObjectsError(missing []MissingObject) error {
	if len(missing) == 0 {
		return errors.New("object store drain is not viable")
	}

	// Keep output deterministic and bounded so users get stable, readable
	// feedback even if there are many missing files.
	sorted := append([]MissingObject(nil), missing...)
	slices.SortFunc(sorted, compareMissingObjects)

	display := sorted
	if len(display) > maxMissingObjectsInError {
		display = display[:maxMissingObjectsInError]
	}

	details := make([]string, 0, len(display))
	for _, entry := range display {
		details = append(
			details,
			fmt.Sprintf("%s:%s (hash=%s)", entry.Namespace, entry.Path, entry.Hash),
		)
	}

	message := fmt.Sprintf(
		"object store drain is not viable on the primary controller: %d files are missing locally; run read-repair before retrying: %s",
		len(sorted),
		strings.Join(details, ", "),
	)
	if len(sorted) > len(display) {
		message = fmt.Sprintf(
			"object store drain is not viable on the primary controller: %d files are missing locally; run read-repair before retrying (showing first %d): %s",
			len(sorted),
			len(display),
			strings.Join(details, ", "),
		)
	}

	return errors.New(message)
}

func uniqueNamespaces(values []string) []string {
	unique := make(map[string]struct{}, len(values))
	for _, value := range values {
		unique[value] = struct{}{}
	}

	result := make([]string, 0, len(unique))
	for value := range unique {
		result = append(result, value)
	}
	slices.Sort(result)
	return result
}

func compareMissingObjects(a, b MissingObject) int {
	if a.Namespace < b.Namespace {
		return -1
	}
	if a.Namespace > b.Namespace {
		return 1
	}
	if a.Path < b.Path {
		return -1
	}
	if a.Path > b.Path {
		return 1
	}
	if a.Hash < b.Hash {
		return -1
	}
	if a.Hash > b.Hash {
		return 1
	}
	return 0
}

// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/collections/transform"

	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/domain/crossmodelrelation"
	crossmodelrelationerrors "github.com/juju/juju/domain/crossmodelrelation/errors"
	"github.com/juju/juju/domain/crossmodelrelation/internal"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

// GetOfferUUID returns the uuid for the provided offer URL.
// Returns crossmodelrelationerrors.OfferNotFound of the offer is not found.
func (s *Service) GetOfferUUID(ctx context.Context, offerURL *crossmodel.OfferURL) (uuid.UUID, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	offerUUID, err := s.modelState.GetOfferUUID(ctx, offerURL.Name)
	if err != nil {
		return uuid.UUID{}, errors.Capture(err)
	}
	return uuid.UUIDFromString(offerUUID)
}

// Offer updates an existing offer, or creates a new offer if it does not exist.
// Permissions are created for a new offer only.
func (s *Service) Offer(
	ctx context.Context,
	args crossmodelrelation.ApplicationOfferArgs,
) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := args.Validate(); err != nil {
		return errors.Capture(err)
	}

	if args.OfferName == "" {
		args.OfferName = args.ApplicationName
	}

	offerUUID, err := uuid.NewUUID()
	if err != nil {
		return errors.Capture(err)
	}
	permissionUUID, err := uuid.NewUUID()
	if err != nil {
		return errors.Capture(err)
	}
	createArgs := internal.MakeCreateOfferArgs(args, offerUUID)

	// Attempt to update the offer, return if successful or an error other than
	// OfferNotFound is received.
	err = s.modelState.UpdateOffer(ctx, createArgs.OfferName, createArgs.Endpoints)
	if err == nil {
		return nil
	} else if !errors.Is(err, crossmodelrelationerrors.OfferNotFound) {
		return errors.Errorf("update offer: %w", err)
	}

	// Verify the owner exists, has not been removed, and
	// is not disabled before creating. Other users can
	// update an offer, such an admin.
	ownerUUID, err := s.controllerState.GetUserUUIDByName(ctx, args.OwnerName)
	if err != nil {
		return errors.Errorf("create offer: %w", err)
	}

	// The offer does not exist, create it.
	err = s.modelState.CreateOffer(ctx, createArgs)
	if err != nil {
		return errors.Errorf("create offer: %w", err)
	}

	err = s.controllerState.CreateOfferAccess(ctx, permissionUUID, offerUUID, ownerUUID)
	if err == nil {
		return nil
	}

	// If we fail to create offer access rows, delete the offer.
	deleteErr := s.modelState.DeleteFailedOffer(ctx, offerUUID)
	if deleteErr != nil {
		err = errors.Join(err, deleteErr)
	}
	err = errors.Errorf("creating access for offer %q: %w", args.OfferName, err)
	return errors.Capture(err)
}

// GetOffers returns offer details for all offers satisfying any of the
// provided filters.
func (s *Service) GetOffers(
	ctx context.Context,
	filters []crossmodelrelation.OfferFilter,
) ([]*crossmodelrelation.OfferDetail, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	details := make([]*crossmodelrelation.OfferDetail, 0)
	var err error
	for _, filter := range filters {
		offerFilterArg := encodeInternalOfferFilter(filter)

		var offerUUIDs []string
		if len(filter.AllowedConsumers) > 0 {
			offerUUIDs, err = s.controllerState.GetOfferUUIDsForUsersWithConsume(ctx, filter.AllowedConsumers)
			if err != nil {
				return nil, errors.Errorf("getting offer UUIDs for allowed consumers: %w", err)
			}
			// If there are no offerUUIDs and nothing else in the filter,
			// move on to the next. The filter has been satisfied.
			if len(offerUUIDs) == 0 && offerFilterArg.Empty() {
				continue
			}
			offerFilterArg.OfferUUIDs = offerUUIDs
		}

		output, err := s.modelState.GetOfferDetails(ctx, offerFilterArg)
		if err != nil {
			return nil, errors.Errorf("getting offer details: %w", err)
		}

		outputWithUsers, err := s.addOfferUsers(ctx, output)
		if err != nil {
			return nil, errors.Errorf("adding allowed consumers: %w", err)
		}

		details = append(details, outputWithUsers...)
	}
	return details, nil
}

func (s *Service) addOfferUsers(
	ctx context.Context,
	input []*crossmodelrelation.OfferDetail,
) ([]*crossmodelrelation.OfferDetail, error) {
	if len(input) == 0 {
		return input, nil
	}

	output := make([]*crossmodelrelation.OfferDetail, 0)

	offerUUIDsForConsumers := transform.Slice(
		input,
		func(in *crossmodelrelation.OfferDetail) string { return in.OfferUUID },
	)

	usersWithConsume, err := s.controllerState.GetUsersForOfferUUIDs(ctx, offerUUIDsForConsumers)
	if err != nil {
		return nil, errors.Errorf("getting offer consumers: %w", err)
	}
	if len(usersWithConsume) == 0 {
		return input, nil
	}

	for _, in := range input {
		users, ok := usersWithConsume[in.OfferUUID]
		if !ok {
			// There are no allowed consumers of the offer.
			continue
		}
		out := in
		out.OfferUsers = users
		output = append(output, out)
	}

	return output, nil
}

func encodeInternalOfferFilter(
	filter crossmodelrelation.OfferFilter,
) internal.OfferFilter {
	return internal.OfferFilter{
		OfferName:              filter.OfferName,
		ApplicationName:        filter.ApplicationName,
		ApplicationDescription: filter.ApplicationDescription,
		Endpoints:              filter.Endpoints,
	}
}

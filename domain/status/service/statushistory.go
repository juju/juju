// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"time"

	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/statushistory"
)

// GetStatusHistory returns the status history based on the request.
func (s *Service) GetStatusHistory(ctx context.Context, request StatusHistoryRequest) (_ []status.DetailedStatus, err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	reader, err := s.statusHistoryReaderFn()
	if err != nil {
		return nil, errors.Errorf("reading status history: %v", err)
	}
	defer reader.Close()

	now := s.clock.Now()

	var results []status.DetailedStatus
	if err := reader.Walk(func(record statushistory.HistoryRecord) (bool, error) {
		// Allow the context to cancel the walk.
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		default:
		}

		// Match the record against the request.
		if ok, err := matches(record, request, now); !ok {
			return false, nil
		} else if err != nil {
			return false, err
		}

		results = append(results, record.Status)

		// If we have more than the requested limit, so we can stop reading.
		if limit := request.Filter.Size; limit > 0 && len(results) >= limit {
			return true, nil
		}

		return false, nil
	}); err != nil {
		return nil, errors.Errorf("reading status history: %w", err)
	}

	return results, nil
}

func matchesUnit(hr statushistory.HistoryRecord, req StatusHistoryRequest) bool {
	switch req.Kind {
	case status.KindUnit:
		return hr.Kind == status.KindUnit || hr.Kind == status.KindUnitAgent || hr.Kind == status.KindWorkload
	case status.KindWorkload:
		return hr.Kind == status.KindWorkload
	case status.KindUnitAgent:
		return hr.Kind == status.KindUnitAgent
	default:
		return false
	}
}

func matches(hr statushistory.HistoryRecord, req StatusHistoryRequest, now time.Time) (bool, error) {
	// Check that the kinds match.
	switch req.Kind {
	case status.KindApplication:
		if hr.Kind != status.KindApplication {
			return false, nil
		}
	case status.KindUnit, status.KindWorkload, status.KindUnitAgent:
		if !matchesUnit(hr, req) {
			return false, nil
		}
	default:
		// TODO: support other kinds.
		return false, errors.Errorf("%q", req.Kind)
	}

	// Check that the tag matches.
	if hr.Tag != req.Tag {
		return false, nil
	}

	filter := req.Filter

	// If the date is set on the filter, check that the record's date is
	// after the filter date.
	if filter.Date != nil && hr.Status.Since != nil && !hr.Status.Since.After(*filter.Date) {
		return false, nil
	}

	// If the delta is set on the filter, check that the record's delta
	// is after the filter delta.
	if filter.Delta != nil && hr.Status.Since != nil && !hr.Status.Since.After(now.Add(-(*filter.Delta))) {
		return false, nil
	}

	return true, nil
}

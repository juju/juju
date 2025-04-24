// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"testing"

	gc "gopkg.in/check.v1"

	corestatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/statushistory"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination package_mock_test.go -source=./service.go
//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination leader_mock_test.go github.com/juju/juju/core/leadership Ensurer

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type statusHistoryRecord struct {
	ns statushistory.Namespace
	s  corestatus.StatusInfo
}

type statusHistoryRecorder struct {
	records []statusHistoryRecord
}

// RecordStatus records the given status information.
// If the status data cannot be marshalled, it will not be recorded, instead
// the error will be logged under the data_error key.
func (r *statusHistoryRecorder) RecordStatus(ctx context.Context, ns statushistory.Namespace, s corestatus.StatusInfo) error {
	r.records = append(r.records, statusHistoryRecord{ns: ns, s: s})
	return nil
}

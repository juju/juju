// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"encoding/json"

	corestatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/domain/status"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/statushistory"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination package_mock_test.go -source=./service.go
//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination service_mock_test.go github.com/juju/juju/domain/status/service StatusHistory,StatusHistoryReader
//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination leader_mock_test.go github.com/juju/juju/core/leadership Ensurer
//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination database_mock_test.go github.com/juju/juju/core/database ClusterDescriber

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

// encodeK8sPodStatus converts a core status info to a db status info.
func encodeK8sPodStatus(s corestatus.StatusInfo) (status.StatusInfo[status.K8sPodStatusType], error) {
	encodedStatus, err := encodeK8sPodStatusType(s.Status)
	if err != nil {
		return status.StatusInfo[status.K8sPodStatusType]{}, err
	}

	var bytes []byte
	if len(s.Data) > 0 {
		var err error
		bytes, err = json.Marshal(s.Data)
		if err != nil {
			return status.StatusInfo[status.K8sPodStatusType]{}, errors.Errorf("marshalling status data: %w", err)
		}
	}

	return status.StatusInfo[status.K8sPodStatusType]{
		Status:  encodedStatus,
		Message: s.Message,
		Data:    bytes,
		Since:   s.Since,
	}, nil
}

// encodeK8sPodStatusType converts a core status to a db cloud container
// status id.
func encodeK8sPodStatusType(s corestatus.Status) (status.K8sPodStatusType, error) {
	switch s {
	case corestatus.Unset:
		return status.K8sPodStatusUnset, nil
	case corestatus.Waiting:
		return status.K8sPodStatusWaiting, nil
	case corestatus.Blocked:
		return status.K8sPodStatusBlocked, nil
	case corestatus.Running:
		return status.K8sPodStatusRunning, nil
	case corestatus.Error:
		return status.K8sPodStatusError, nil
	default:
		return -1, errors.Errorf("unknown cloud container status %q", s)
	}
}

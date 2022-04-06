package grpcserver

import (
	"context"

	"github.com/juju/juju/worker/grpcserver/simpleapi"
)

// This implements the SimpleService gRPC service.
type server struct {
	simpleapi.UnimplementedSimpleServiceServer
}

func (s *server) Status(ctx context.Context, req *simpleapi.StatusRequest) (*simpleapi.StatusResponse, error) {
	client, err := getClient(ctx)
	if err != nil {
		return nil, err
	}
	status, err := client.Status(req.Patterns)
	if err != nil {
		return nil, err
	}
	applications := map[string]*simpleapi.ApplicationStatus{}
	for name, status := range status.Applications {
		applications[name] = &simpleapi.ApplicationStatus{
			CharmName:    status.Charm,
			CharmVersion: status.CharmVersion,
			Status: &simpleapi.DetailedStatus{
				Status: status.Status.Status,
				Info:   status.Status.Info,
				Life:   string(status.Status.Life),
			},
		}
	}
	return &simpleapi.StatusResponse{
		Model: &simpleapi.ModelStatusInfo{
			Name:         status.Model.Name,
			Type:         status.Model.Type,
			Applications: applications,
		},
	}, nil
}

func (s *server) Deploy(ctx context.Context, req *simpleapi.DeployRequest) (*simpleapi.DeployResponse, error) {
	conn, err := getConnection(ctx)
	if err != nil {
		return nil, err
	}
	numUnits := int(req.NumUnits)
	if numUnits == 0 {
		numUnits = 1
	}

	err = deployCharm(conn, deployCharmArgs{
		CharmName:       req.CharmName,
		ApplicationName: req.ApplicationName,
		NumUnits:        numUnits,
		Revision:        -1,
	})
	if err != nil {
		return nil, err
	}
	return &simpleapi.DeployResponse{}, nil
}

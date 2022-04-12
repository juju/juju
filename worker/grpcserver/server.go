package grpcserver

import (
	"context"

	apiapplication "github.com/juju/juju/api/client/application"
	"github.com/juju/juju/cmd/juju/block"
	simpleapi "github.com/juju/juju/grpc/gen/proto/go/juju/client/simple/v1alpha1"
	"github.com/juju/juju/rpc/params"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// This implements the SimpleService gRPC service.
type server struct {
	simpleapi.UnimplementedSimpleServiceServer

	controllerAddresses []string
}

func (s *server) Status(ctx context.Context, req *simpleapi.StatusRequest) (*simpleapi.StatusResponse, error) {
	client, err := s.getClient(ctx)
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
	conn, err := s.getConnection(ctx)
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

func (s *server) RemoveApplication(ctx context.Context, req *simpleapi.RemoveApplicationRequest) (*simpleapi.RemoveApplicationResponse, error) {
	conn, err := s.getConnection(ctx)
	if err != nil {
		return nil, err
	}
	client := apiapplication.NewClient(conn)
	results, err := client.DestroyApplications(apiapplication.DestroyApplicationsParams{
		Applications:   []string{req.ApplicationName},
		DestroyStorage: req.DestroyStorage,
		Force:          req.Force,
	})
	if err := block.ProcessBlockedError(err, block.BlockRemove); err != nil {
		return nil, err
	}
	if len(results) != 1 {
		return nil, status.Error(codes.Internal, "Internal error")
	}
	result := results[0]
	if result.Error != nil {
		return nil, result.Error
	}
	return &simpleapi.RemoveApplicationResponse{
		DestroyedStorageTags: entitySliceToTagSlice(result.Info.DestroyedStorage),
		DetachedStorageTags:  entitySliceToTagSlice(result.Info.DetachedStorage),
		DestroyedUniTagss:    entitySliceToTagSlice(result.Info.DestroyedUnits),
	}, nil
}

func entitySliceToTagSlice(entities []params.Entity) []string {
	tags := make([]string, len(entities))
	for i, e := range entities {
		tags[i] = e.Tag
	}
	return tags
}

package service

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/core/crossmodel"
)

type State interface {
	UpdateExternalController(ctx context.Context, ec crossmodel.ControllerInfo, modelUUIDs []string) error
}

type Service struct {
	st State
}

func NewService(st State) *Service {
	return &Service{st}
}

func (s *Service) UpdateExternalController(
	ctx context.Context, ec crossmodel.ControllerInfo, modelUUIDs ...string,
) error {
	err := s.st.UpdateExternalController(ctx, ec, modelUUIDs)
	return errors.Annotate(err, "updating external controller state")
}

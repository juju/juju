



type ContextFunc func(interface{}) error

type Operation interface {
	Run(stop <-chan struct{}, getContext ContextFunc) error
}

type Serializer interface {
	worker.Worker
	Send(op Operation) error
}

func NewSerializer(getContext ContextFunc) Serializer {
	s := &serializer{
		ops: make(chan Operation),
	}
	go func() {
		defer s.tomb.Done()
		s.tomb.Kill(s.loop(getContext))
	}()
	return s
}

type serializer struct {
	tomb tomb.Tomb
	ops  chan Operation
}

func (s *serializer) Send(op Operation) error {
	select {
	case <-s.tomb.Dying():
		return errStopped
	case s.ops <- op:
		return nil
	}
}

func (s *serializer) loop(getContext ContextFunc) error {
	for {
		select {
		case <-s.tomb.Dying():
			return tomb.ErrDying
		case op := <-s.ops:
			if err := op.Run(getContext, s.tomb.Dying()); err != nil {
				return errors.Trace(err)
			}
		}
	}
}

// Collection contains three kinds of documents, with names matching the
// following pattern:
// config
// <env>#writers
// <env>#<namespace>#lease

package lease

type Collection interface {
	//...
}
type Closer func()
type GetCollection func(string) (Collection, Closer)
type RunTransaction func(jujutxn.TransactionSource) error

func NewManager() Manager {

}

type manager struct {
	tomb    tomb.Tomb
	storage *storage
	claims  chan claim
}

func (m *manager) ClaimLease(namespace, holder string, duration time.Duration) error {
	response := make(chan bool)

	select {
	case <-m.tomb.Dying():
		return worker.ErrStopped
	case m.claims <- claim{namespace, holder, duration, response}:
	}

	select {
	case <-m.tomb.Dying():
		return worker.ErrStopped
	case success := <-response:
		if !success {
			return lease.ErrClaimDenied
		}
	}
	return nil
}

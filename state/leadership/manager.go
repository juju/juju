// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership

type ManagerConfig struct {
	LeaseClient lease.Client
	Watcher     Watcher
	Clock       Clock
}

type manager struct {
	Serializer
}

func NewManager(leaseClient lease.Client, watcher *watcher.Watcher) Manager {
	getContext := func(out interface{}) error {
		outPtr, ok := out.(*lease.Client)
		if !ok {
			return errors.Errorf("expected *lease.Client, got %T", out)
		}
		*outPtr = client
		return nil
	}
	return &manager{NewSerializer(getContext)}
}

func (manager *manager) ClaimLeadership(serviceName, unitName string, duration time.Duration) error {
	claim := &claim{
		serviceName: serviceName,
		unitName:    unitName,
		duration:    duration,
		response:    make(chan error),
	}
	if err := manager.Send(claim); err != nil {
		return nil, errors.Trace(err)
	}
	return claim.wait(manager.tomb.Dying())
}

type claim struct {
	serviceName string
	unitName    string
	duration    time.Duration
	response    chan<- error
}

func (claim *claim) Run(getContext ContextFunc, stop chan<- struct{}) error {
	var client lease.Client
	if err := getContext(&client); err != nil {
		return nil, errors.Trace(err)
	}
	for {
		select {
		case <-stop:
			return errStopped
		default:
			switch err := claim.run(client); err {
			case nil, leadership.ErrClaimDenied:
				return claim.respond(err)
			case lease.ErrInvalid:
				// client out of date; cache has been refreshed, go round again
			default:
				return errors.Trace(err)
			}
		}
	}
}

func (claim *claim) run(client lease.Client) error {
	info, found := client.Leases()[claim.serviceName]
	if !found {
		return client.ClaimLease(claim.serviceName, claim.unitName, claim.duration)
	}
	if info.Holder == claim.unitName {
		return client.ExtendLease(claim.serviceName, claim.unitName, claim.duration)
	}
	return leadership.ErrClaimDenied
}

func (claim *claim) respond(result error, stop chan<- struct{}) error {
	select {
	case <-stop:
		return tomb.ErrDying
	case claim.ressponse <- result:
		return nil
	}
}

func (claim *claim) wait(stop <-chan struct{}) error {
	select {
	case <-stop:
		return errStopped
	case err := <-claim.response:
		return errors.Trace(err)
	}
}

type check struct {
	serviceName string
	unitName    string
	response    chan<- checkResult
}

type checkResult struct {
	token Token
	err   error
}

func (manager *manager) CheckLeadership(serviceName, unitName string) (Token, error) {
	response := make(chan checkResult)
	check := &check{serviceName, unitName, response}
	select {
	case <-manager.tomb.Dying():
		return errStopped
	case manager.checks <- check:
	}
	select {
	case <-manager.tomb.Dying():
		return errStopped
	case result := <-response:
		if result.err != nil {
			return nil, errors.Trace(result.err)
		}
		return result.token, nil
	}
}

func (manager *manager) WatchLeaderless(serviceName string) (NotifyWatcher, error) {
	panic("not done")
}

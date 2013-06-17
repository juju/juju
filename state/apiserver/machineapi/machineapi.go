package machineapi

type Root struct {
	client *state.Machine
	*machiner.Root
	*watcherapi.EntityWatcherRoot
	*watcherapi.LifecycleWatcherRoot
	*watcherapi.EnvironConfigWatcherRoot
}

// Machiner returns an object that provides access to the Machiner API.
// The id argument is reserved for future use and currently
// needs to be empty.
func (r *Root) Machiner(id string) (*machiner.Machiner, error) {
	if id != "" {
		// Safeguard id for possible future use.
		return nil, common.ErrBadId
	}
	return machiner.New(r.srv.state, r)
}

func (r *Root) User(name string) (*user.User, error) {
	return commonapi.NewUser(client, name)
}

func (r *Root) EntityWatcher(id string) (*watcherapi.EntityWatcher, error) {
	return r.
}
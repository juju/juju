package state

// ReapEnviron checks if there are any living machines or services left in a
// dying environment. If there are, it sets them to dying.
// func (st *State) ReapEnviron() (err error) {

// 	machines, err := st.AllMachines()
// 	if err != nil {
// 		return nil, errors.Trace(err)
// 	}

// 	for _, machine := range machines {
// 		destroyMachineOps, err := machine.forceDestroyOps()
// 		if err != nil {
// 			if isManagerMachineError(err) {
// 				continue
// 			}
// 			return nil, errors.Trace(err)
// 		}
// 		destroyOps = append(destroyOps, destroyMachineOps...)
// 	}

// 	// TODO(waigani) Do we need this? A cleanup job to destroy all
// 	// services was added when the environment was destroyed.

// 	buildTxn := func(attempt int) ([]txn.Op, error) {
// 		env, err := st.Environment()
// 		if err != nil {
// 			return nil, errors.Trace(err)
// 		}
// 		if attempt > 0 {
// 			err := env.Refresh()
// 			if err != nil {
// 				return nil, errors.Trace(err)
// 			}
// 		}

// 		if env.Life() != Dying {
// 			return nil, errors.New("environment is not dying")
// 		}

// 		var destroyOps []txn.Op
// 		machines, err := st.AllMachines()
// 		if err != nil {
// 			return nil, errors.Trace(err)
// 		}

// 		for _, machine := range machines {
// 			destroyMachineOps, err := machine.forceDestroyOps()
// 			if err != nil {
// 				if isManagerMachineError(err) {
// 					continue
// 				}
// 				return nil, errors.Trace(err)
// 			}
// 			destroyOps = append(destroyOps, destroyMachineOps...)
// 		}

// 		// TODO(waigani) Do we need this? A cleanup job to destroy all
// 		// services was added when the environment was destroyed.
// 		services, err := st.AllServices()
// 		if err != nil {
// 			return nil, errors.Trace(err)
// 		}
// 		for _, service := range services {
// 			destroyServiceOps, err := service.destroyOps()
// 			if err != nil {
// 				return nil, errors.Trace(err)
// 			}
// 			destroyOps = append(destroyOps, destroyServiceOps...)
// 		}
// 		xxx.Dump(destroyOps)
// 		return destroyOps, nil
// 	}

// 	if err = st.run(buildTxn); err != nil {
// 		return errors.Trace(err)
// 	}
// 	return nil
// }

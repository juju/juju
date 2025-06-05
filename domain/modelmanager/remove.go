// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager

// RemoveModelOptions is a struct that is used to modify the behavior of
// removing a non-activated model.
type RemoveModelOptions struct {
	deleteDB bool
}

// DeleteDB returns a boolean value that indicates if the model database should
// be removed.
func (o RemoveModelOptions) DeleteDB() bool {
	return o.deleteDB
}

// DefaultRemoveModelOptions returns a pointer to a [RemoveModelOptions] struct
// with the default values set.
func DefaultRemoveModelOptions() *RemoveModelOptions {
	return &RemoveModelOptions{
		// Until we have correctly implemented tearing down the model, we want
		// to keep the model around during normal model removal.
		deleteDB: false,
	}
}

// RemoveModelOption is a functional option that can be used to modify the
// behavior of removing a non activated model.
type RemoveModelOption func(*RemoveModelOptions)

// WithDeleteDB is a functional option that can be used to modify the behavior
// of the RemoveModel function to delete the model database.
func WithDeleteDB() RemoveModelOption {
	return func(o *RemoveModelOptions) {
		o.deleteDB = true
	}
}

// WithoutDeleteDB is a functional option that can be used to modify the
// behavior of the RemoveModel function to not delete the model database.
func WithoutDeleteDB() RemoveModelOption {
	return func(o *RemoveModelOptions) {
		o.deleteDB = false
	}
}

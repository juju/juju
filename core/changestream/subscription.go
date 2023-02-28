// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package changestream

// Handler describes a function for handling a new ChangeEvent.
type Handler = func(ChangeEvent)

// Subscription is an interface that can be used to receive events from the
// event queue and unsubscribe from the queue.
type Subscription interface {
	// Unsubscribe removes the subscription from the event queue.
	Unsubscribe()
}

// SubscriptionOption is an option that can be used to create a subscription.
type SubscriptionOption struct {
	namespace  string
	changeMask ChangeType
	filter     func(ChangeEvent) bool
}

// Namespace returns the name of the type that the subscription will tied to.
func (o SubscriptionOption) Namespace() string {
	return o.namespace
}

// ChangeMask returns the change mask that the subscription will be for.
func (o SubscriptionOption) ChangeMask() ChangeType {
	return o.changeMask
}

// FilterFn returns the filter function that the subscription will be for.
func (o SubscriptionOption) Filter() func(ChangeEvent) bool {
	return o.filter
}

// Namespace returns a SubscriptionOption that will subscribe to the given
// namespace.
func Namespace(namespace string, changeMask ChangeType) SubscriptionOption {
	return SubscriptionOption{
		namespace:  namespace,
		changeMask: changeMask,
		filter:     func(ce ChangeEvent) bool { return true },
	}
}

// FilteredNamespace returns a SubscriptionOption that will subscribe to the given
// topic and filter the events using the given function.
func FilteredNamespace(namespace string, changeMask ChangeType, filter func(ChangeEvent) bool) SubscriptionOption {
	opt := Namespace(namespace, changeMask)
	opt.filter = filter
	return opt
}

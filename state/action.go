// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/juju/names"
	"labix.org/v2/mgo/txn"
)

// ActionReceiver describes objects that can have actions queued for them, and
// that can get ActionRelated information about those actions.
// TODO(jcw4) consider implementing separate Actor classes for this interface;
// for example UnitActor that implements this interface, and takes a Unit and
// performs all these actions.
type ActionReceiver interface {
	// AddAction queues an action with the given name and payload for this
	// ActionReciever
	AddAction(name string, payload map[string]interface{}) (*Action, error)

	// WatchActions returns a StringsWatcher that will notify on changes to the
	// queued actions for this ActionReceiver
	WatchActions() StringsWatcher

	// Actions returns the list of Actions queued for this ActionReceiver
	Actions() ([]*Action, error)

	// ActionResults returns the list of completed ActionResults that were
	// queued on this ActionReciever
	ActionResults() ([]*ActionResult, error)

	// ActionKey returns the unique prefix that will be used to filter actions
	// that are queued for this ActionReceiver
	ActionKey() string
}

var (
	_ ActionReceiver = (*Unit)(nil)
	// TODO(jcw4) - use when Actions can be queued for Services
	//_ ActionReceiver = (*Service)(nil)
)

const actionMarker string = "#a#"

type actionDoc struct {
	// Id is the key for this document. The structure of the key is
	// a composite of ActionReciever.ActionKey() and a unique sequence,
	// to facilitate indexing and prefix filtering
	Id string `bson:"_id"`

	// Name identifies the action that should be run; it should
	// match an action defined by the unit's charm.
	Name string

	// Payload holds the action's parameters, if any; it should validate
	// against the schema defined by the named action in the unit's charm
	Payload map[string]interface{}
}

// Action represents an instruction to do some "action" and is expected
// to match an action definition in a charm.
type Action struct {
	st  *State
	doc actionDoc
}

// newAction builds an Action for the given State and actionDoc
func newAction(st *State, adoc actionDoc) *Action {
	return &Action{
		st:  st,
		doc: adoc,
	}
}

// newActionDoc builds the actionDoc with the given name and parameters
func newActionDoc(st *State, ar ActionReceiver, actionName string, parameters map[string]interface{}) (actionDoc, error) {
	prefix := actionPrefix(ar)
	seq, err := st.sequence(prefix)
	if err != nil {
		return actionDoc{}, err
	}
	return actionDoc{Id: actionId(prefix, seq), Name: actionName, Payload: parameters}, nil
}

// actionPrefix returns the expected action prefix for a given ActionReceiver
func actionPrefix(r ActionReceiver) string {
	return r.ActionKey() + actionMarker
}

// actionPrefixFromUnitTag builds the prefix of an action with a given unit id
func actionPrefixFromUnitId(unitId string) string {
	return unitGlobalKey(unitId) + actionMarker
}

// actionId builds the id from the prefix and suffix
func actionId(prefix string, suffix int) string {
	return fmt.Sprintf("%s%d", prefix, suffix)
}

// Id returns the id of the Action
func (a *Action) Id() string {
	return a.doc.Id
}

// Tag implements the Entity interface and returns a names.Tag that
// is a names.ActionTag
func (a *Action) Tag() names.Tag {
	return a.ActionTag()
}

// UnitName extracts the name of the unit from the encoded _id
// and true if successful, or false if this action does not have
// a unit name in the _id
func (a *Action) UnitName() string {
	name, ok := extractPrefixName(a.doc.Id, "u#")
	if !ok {
		// TODO(jcw4) this will go away once we refactor names.ActionTag
		// to accept units or services
		panic(fmt.Sprintf("cannot extract unit name from _id %v", a.doc.Id))
	}
	return name
}

// Sequence extracts the unique sequence part of an action _id
func (a *Action) Sequence() int {
	sequence, ok := extractSequence(a.doc.Id)
	if !ok {
		panic(fmt.Sprintf("cannot extract sequence from _id %v", a.doc.Id))
	}
	return sequence
}

func extractSequence(id string) (int, bool) {
	parts := strings.SplitN(id, actionMarker, 2)
	if len(parts) != 2 {
		return -1, false
	}
	parsed, err := strconv.ParseInt(parts[1], 10, 0)
	if err != nil {
		return -1, false
	}
	return int(parsed), true
}

func extractPrefixName(id, prefix string) (string, bool) {
	plen := len(prefix)
	mlen := len(actionMarker)
	// id must contain the prefix and the actionMarker plus
	// two more characters at the very minimum
	if len(id) <= plen+mlen+2 || id[:plen] != prefix {
		return "", false
	}
	parts := strings.Split(id, actionMarker)
	if len(parts) != 2 {
		return "", false
	}
	return parts[0][plen:], true
}

// ActionTag returns an ActionTag constructed from this action's
// OwnerName and Sequence
func (a *Action) ActionTag() names.ActionTag {
	// TODO(jcw4) we're assuming only Units right now
	unitTag := names.NewUnitTag(a.UnitName())
	actionTag := names.NewActionTag(unitTag, a.Sequence())
	return actionTag
}

// Name returns the name of the action, as defined in the charm
func (a *Action) Name() string {
	return a.doc.Name
}

// Payload will contain a structure representing arguments or parameters to
// an action, and is expected to be validated by the Unit using the Charm
// definition of the Action
func (a *Action) Payload() map[string]interface{} {
	return a.doc.Payload
}

// Complete removes action from the pending queue and creates an ActionResult
// to capture the output and end state of the action.
func (a *Action) Complete(output string) error {
	return a.removeAndLog(ActionCompleted, output)
}

// Fail removes an Action from the queue, and creates an ActionResult that
// will capture the reason for the failure.
func (a *Action) Fail(reason string) error {
	return a.removeAndLog(ActionFailed, reason)
}

// removeAndLog takes the action off of the pending queue, and creates an
// actionresult to capture the outcome of the action.
func (a *Action) removeAndLog(finalStatus ActionStatus, output string) error {
	doc := newActionResultDoc(a, finalStatus, output)
	return a.st.runTransaction([]txn.Op{
		addActionResultOp(a.st, &doc),
		{
			C:      a.st.actions.Name,
			Id:     a.doc.Id,
			Remove: true,
		},
	})
}

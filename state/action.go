package state

type ActionStatus string

const (
	ActionPending ActionStatus = "pending"
	ActionRunning ActionStatus = "running"
)

type actionDoc struct {
	Id      string `bson:"_id"`
	Name    string
	Payload string
	Status  ActionStatus
}

type Action struct {
	doc actionDoc
}

func newAction(adoc *actionDoc) *Action {
	action := &Action{
		doc: *adoc,
	}
	return action
}

func (a *Action) Name() string {
	return a.doc.Name
}

func (a *Action) Id() string {
	return a.doc.Id
}

func (a *Action) Payload() string {
	return a.doc.Payload
}

func (a *Action) Status() ActionStatus {
	return a.doc.Status
}

func (a *Action) setRunning() {
	a.doc.Status = ActionRunning
}

func (a *Action) setPending() {
	a.doc.Status = ActionPending
}

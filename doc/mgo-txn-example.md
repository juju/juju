<!-- TODO(gfouillet): do not merge into 4.0, or delete whenever merged (reason: related to mongodb) -->

The following transaction is an exemplar of the principles outlined in docs/hacking-state.txt:

```go
func (e *Environment) Destroy() (err error) {
defer errors.DeferredAnnotatef(&err, "failed to destroy environment")
// create a local variable so we can get a newer version in case we need to retry.
env := e
buildTxn := func (attempt int) ([]txn.Op, error) {

// On the first attempt, we assume memory state is recent
// enough to try using...
if attempt != 0 {
// ...but on subsequent attempts, we read fresh environ
// state from the DB. Note that we do *not* refresh `e`
// itself, as detailed in doc/hacking-state.txt
if env, err = env.st.Environment(); err != nil {
return nil, errors.Trace(err)
}
}

// It's nice to keep the actual destruction logic separate
// from the coordinating jiggery-pokery in this method.
ops, err := env.destroyOps()
if err == errEnvironNotAlive {
// Not a big deal in any way. This might be the second
// attempt, following up from a failure-that-actually-
// succeeded on the first; or it might be the first
// attempt, and we're observing that someone else
// already destroyed it; or any one of a bunch of other
// scenarios.
return nil, jujutxn.ErrNoOperations
} else if err != nil {
return nil, errors.Trace(err)
}

return ops, nil
}
return env.st.run(buildTxn)
}

// errEnvironNotAlive is a signal emitted from destroyOps to indicate
// that environment destruction is already underway. It should not
// escape this package.
var errEnvironNotAlive = errors.New("environment is no longer alive")

// destroyOps returns the txn operations necessary to begin environ
// destruction, or an error indicating why it can't.
func (e *Environment) destroyOps() ([]txn.Op, error) {

// Now this method can just always assume that e is up to date.
// Every check we make can be against memory state...
if e.Life() != Alive {
return nil, errEnvironNotAlive
}
uuid := e.UUID()
ops := []txn.Op{{
C:  environmentsC,
Id: uuid,
// ...and corresponds to a txn Assert which ensures the
// checked state still appplies when the txn runs.
Assert: isEnvAliveDoc,
Update: bson.D{{"$set", bson.D{{"life", Dying}}}},
}}

// We can follow the same approach with the host/hosted bits:
if uuid == e.doc.ServerUUID {
// So if we're a host, we can read the db into memory to
// check for hosted envs...
if count, err := hostedEnvironCount(e.st); err != nil {
return nil, errors.Trace(err)
} else if count != 0 {
// ...and return an error in this one place...
return nil, errors.New("hosting %d other environments", count)
}
// ...or add an assertion to make sure the count is
// still 0.
ops = append(ops, assertNoHostedEnvironsOp())
} else {
// When we're destroying a hosted environment, no further
// checks are necessary -- we just need to make sure we
// update the refcount.
ops = append(ops, decHostedEnvironCountOp())
}

// Because txn operations execute in order, and may encounter
// arbitrarily long delays, we need to make sure every op
// causes a state change that's still consistent; so we make
// sure the cleanup op is the last thing that will execute.
cleanupOp := e.st.newCleanupOp(cleanupServicesForDyingEnvironment, uuid)
ops = append(ops, cleanupOp)
return ops, nil
}
```
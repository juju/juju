Test Failure FAQ
================

This is an attempt to create a resource for weird test failures.

### test times out

Almost certainly there is a deadlock in the code, or the test is actually
using time.Sleep for a long value. You need to fix the test.


### debug-log handler error: tailer stopped: tailable cursor requested on non capped collection

Now that the logs are kept in mongo, the debug log code attempts to tail
the oplog to get the new logs. Most tests don't set up mongo in replicaset
mode, so no oplog collection is created. Most debug-log tests now mock out
the oplog with a capped collection for testing purposes.

If you hit this error, you need to either provide a mock oplog, or don't call
debug-log.


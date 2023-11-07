juju-txn-helper: a tool for reviewing transaction queues on models
Copyright Canonical Ltd.
License: GNU Affero General Public License version 3


What this does:

For now, this repository contains a single script, txn_helper.py.

This script walks through a subset of Juju's transactions collection ("txns"),
examining each operation, testing each operation's assertions and providing
details about each failed assertion, as well as details about the proposed
database modifications.

This script has two modes:

* The default mode is for this script to examine the entire transactions
  collection, filtering by a specific integer state code, e.g. 5 for the
  ABORTED state, which is the default filter. The specific filter can be
  modified via the --state argument.  Supported codes can be found by examining
  the OpState enumeration in the source code.

* If a model name or UUID is specified, the script will examine all
  transactions referenced in the txn-queue field of the specified model's
  document.  In this case, the --state filter has no effect; it is assumed that
  the full set of queries in the txn-queue field provide important context.


Important caveat: This script must be run against the MongoDB primary.


Snap support:

This repository also includes a snapcraft.yaml file, and can be built
to a snap via running "snapcraft".  The snap has been tested
successfully on Ubuntu Xenial through Focal.


Usage:

A typical live Juju MongoDB instance will typically require an
invocation like this:

  python3 txn_helper.py -s -H mongodb://127.0.0.1:37017 -u $user -p $password

If using the snap, this would change to:

  juju-txn-helper -s -H mongodb://127.0.0.1:37017 -u $user -p $password

See --help for additional helpful options.  Some suggestions are
-d/--dump-transaction and -P/--include-passes.

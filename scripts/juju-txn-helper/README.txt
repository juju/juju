juju-txn-helper: a tool for reviewing transaction queues on models
Copyright Canonical Ltd.
License: GNU Affero General Public License version 3


What this does:

For now, this repository contains a single script, txn_helper.py.

Given a model UUID, this script walks through each transaction's
operations, tests each operation's assertions and provides details
about each failed assertion.  It also provides details about
insert/update records as well as records already existing in the
database.


Important caveats:

* The script does not filter out any transactions which have already
  completed; it literally just goes through the full list of
  transactions specified in a model's txn-queue field.

* This script must be run against the MongoDB primary.


Snap support:

This repository also includes a snapcraft.yaml file, and can be built
to a snap via running "snapcraft".  The snap has been tested
successfully on Ubuntu Xenial through Focal.


Usage:

A typical live Juju MongoDB instance will typically require an
invocation like this:

  python3 txn_helper.py -s -H mongodb://127.0.0.1:37017 -u $user -p $password $MODEL

If using the snap, this would change to:

  juju-txn-helper -s -H mongodb://127.0.0.1:37017 -u $user -p $password $MODEL

See --help for additional helpful options.  Some suggestions are
-d/--dump-transaction and -P/--include-passes.

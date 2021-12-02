# juju-leasediff

Juju Lease diff is intended to analyse the lease reports from a Juju controller.
You can feed it multiple lease reports. It will tell you if a split-brain of
the underlying raft FSM has occurred.

### Run

Running it is extremely simple:

```sh
make install
juju-leasediff report_1 report_2 report_3
```
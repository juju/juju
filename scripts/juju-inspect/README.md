# juju-inspect

Juju inspect is intended to analyse engine reports from a Juju controller. You
can feed it multiple engine reports and it will tell you various information
about the state of the controllers.

### Run

Running it is extremely simple:

```sh
go run main.go report_1 report_2 report_3
```

The output should look like:

```sh
Analysis of Engine Report:

Raft Leader:
        machine-2 is the leader.

Mongo Primary:
        There are no primaries found.

Pubsub Forwarder:
        machine-0 is connected to the following: [machine-1]
        machine-1 is connected to the following: [machine-0]
        machine-2 is connected to the following: [machine-0 machine-1]
```
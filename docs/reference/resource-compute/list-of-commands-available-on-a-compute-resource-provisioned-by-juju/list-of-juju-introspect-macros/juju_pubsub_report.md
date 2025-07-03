(juju_pubsub_report)=
# `juju_pubsub_report`

`juju_pubsub_report` shows the pubsub connections originating from a Juju controller.
The report includes details on the message queues and is useful for diagnostics.

## Usage
Must be run on a Juju controller machine.
```code
juju_presence_report
```

## Example output
```text
$ juju_pubsub_report 
PubSub Report:

Source: machine-2

Target: machine-0
  Status: connected
  Addresses: [10.213.99.145:17070]
  Queue length: 0
  Sent count: 9

Target: machine-3
  Status: connected
  Addresses: [10.213.99.125:17070]
  Queue length: 0
  Sent count: 10
```

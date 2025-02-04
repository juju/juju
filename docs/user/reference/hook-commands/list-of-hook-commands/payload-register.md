(hook-command-payload-register)=
# `payload-register`

## Summary
Register a charm payload with Juju.

## Usage
``` payload-register [options] <type> <class> <id> [tags...]```

## Examples

    payload-register monitoring docker 0fcgaba


## Details

"payload-register" is used while a hook is running to let Juju know that a
payload has been started. The information used to start the payload must be
provided when "register" is run.

The payload class must correspond to one of the payloads defined in
the charm's metadata.yaml.

An example fragment from metadata.yaml:

payloads:
    monitoring:
        type: docker
    kvm-guest:
        type: kvm
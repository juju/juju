# juju-qa-test

## Description

A container-based V2 metadata charm to use in testing juju.

## Usage

Basic deploy:
`juju deploy juju-qa-test`

Set the unit status to the first line of the resource, if available, one time:
`juju config juju-qa-test foo-file=true`

Get your fortune
`juju run-action juju-qa-test/0 fortune`


## Version, Channel, Series and History
| Version    | Revision | Channel          | Series                               |
| ---------- | -------- | ---------------- | ------------------------------------ |
| 1.1-stable | 26       | latest/stable    | focal, bionic, xenial                |
| 1.4-cand   | 20       | latest/candidate | jammy, focal, bionic, xenial         |
| 2.0-edge   | 21       | latest/edge      | groovy, jammy, focal, bionic, xenial |
| 2.0-stable | 22       | 2.0/stable       | disco, bionic, xenial, trusty        |
| 2.0-edge   | 23       | 2.0/edge         | disco, bionic, xenial, trusty        |
| 3.0-stable | 27       | 2.3/stable       | jammy                                |

To publish new versions of stable/candidate/edge see the files in the
subdirectories 'stable', 'candidate', 'edge' respectively. You should be able
to copy those files into this directory, run `charcmraft pack; charcmcraft upload`,
and then update this file with the new revisions.

### Test that depend on juju-qa-test

If you need to roll out new revisions, make sure to do a grep for tests that interact with `juju-qa-test`.
Currently those are:

```
  suites/resources/basic.sh
    needs the resources to be available and matching the test expectations (so
    you can deploy stable or candidate and see what the resource content is, to
    make sure we get the right resource associated with the given track)
  suites/resources/upgrades.sh
    uses multiple channels to ensure that we can 'refresh' to switch to
    different versions of the resource.
  suites/deploy/deploy_bundles.sh
    has a bundle (juju-qa-test-bundle) that deploys several charms including
    juju-qa-test and ensure that juju can deploy a charm from a bundle with an
    explicitly listed channel
  suites/deploy/deploy_revision.sh
    tests that we can deploy an explicit revision from charmhub, this
    especially needs care as new revisions need the test to be updated to
    match, it also associates explicit resource revisions with those channels
```


## Resource versions and contents

Use `juju charm-resources juju-qa-test --channel <channel>` to determine resource to charm channel correlation.

| Revision | File Contents         | Notes                                           |
| -------- | --------------------- | ----------------------------------------------- |
| 1        | testing one.          |                                                 |
| 2        | testing two.          |                                                 |
| 3        | testing one plus one. | Will be used to replace Revision 1 in a channel |
| 4        | testing four.         |                                                 |


## Deployment

It is expected that you have charmcraft installed via

`snap install charmcraft`

Then cd in to testcharms/charm-hub/charms/juju-qa-test and run

```bash
charmcraft clean
make stable
juju deploy juju-qa-test.charm
```

## Releasing

When releasing, use `juju charm-resources` as above to make sure you affiliate the charm with the correct resource.
You can then do something like:

```bash
charmcraft release juju-qa-test --revision 19 --channel latest/stable --resource foo-file:2
charmcraft release juju-qa-test --revision 20 --channel latest/candidate --resource foo-file:4
charmcraft release juju-qa-test --revision 21 --channel latest/edge --resource foo-file:4
```

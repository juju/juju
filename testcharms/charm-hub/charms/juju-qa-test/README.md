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
| 1.1-stable | 19       | latest/stable    | focal, bionic, xenial                |
| 1.4-cand   | 20       | latest/candidate | jammy, focal, bionic, xenial         |
| 2.0-edge   | 21       | latest/edge      | groovy, jammy, focal, bionic, xenial |
| 8          | 8        | 2.0/stable       | disco, bionic, xenial, trusty        |
| 10         | 10       | 2.0/edge         | disco, bionic, xenial, trusty        |

To publish new versions of stable/candidate/edge see the files in the
subdirectories 'stable', 'candidate', 'edge' respectively. You should be able
to copy those files into this directory, run `charcmraft pack; charcmcraft upload`,
and then update this file with the new revisions.


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
charmcraft pack
juju deploy juju-qa-test.charm
```

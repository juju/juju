# juju-qa-test

## Description

A container-based V2 metadata charm to use in testing juju.

## Usage

Basic deploy: <br>
<code>juju deploy juju-qa-test</code>

Set the unit status to the first line of the resource, if available, one time:<br>
<code>juju config juju-qa-test foo-file=true</code>

Get your fortune<br>
<code>juju run-action juju-qa-test/0 fortune</code>


## Version, Channel, Series and History
|Version  |Channel          |Series				 
|----      |----            |----               
| 11|latest/stable|bionic, xenial|
| 14|latest/candidate|focal, bionic, xenial|
| 20|latest/edge|groovy, focal, bionic, xenial|
|8   |2.0/stable  |disco, bionic, xenial, trusty |
|10      |2.0/edge   |disco, bionic, xenial, trusty   |

## Resource versions and contents

Use `juju charm-resources juju-qa-test --channel <channel>` to determine resource to charm channel correlation.

|Revision  |File Contents  | Notes
|----      |----  |----
|1      |testing one.                     
|2      |testing two. 
|3      |testing one plus one. | Will be used to replace Revision 1 in a channel
| 4 | testing four.


## Deployment

It is expected that you have charmcraft installed via

<code>snap install charmcraft</code>

Then cd in to testcharms/charm-hub/charms/juju-qa-test and run

<code>
charmcraft build
juju deploy juju-qa-test.charm
</code>

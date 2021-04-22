# juju-qa-test-v2

## Description

A non-container-based V2 metadata charm to use in testing juju.

## Usage

Basic deploy: <br>
<code>juju deploy juju-qa-test-v2</code>

Set the unit status to the first line of the resource, if available, one time:<br>
<code>juju config juju-qa-test-v2 foo-file=true</code>

Get your fortune<br>
<code>juju run-action juju-qa-test-v2/0 fortune</code>


## Deployment

It is expected that you have charmcraft installed via

<code>snap install charmcraft</code>

Then cd in to testcharms/charm-hub/charms/juju-qa-test-v2 and run

<code>
charmcraft build
juju deploy juju-qa-test-v2.charm
</code>

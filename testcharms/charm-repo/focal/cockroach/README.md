# cockroachdb

## Description

A container-based V2 metadata kubernetes charm to use in testing juju.

## Deployment

Pull a cockroachdb image to be used from https://hub.docker.com/r/cockroachdb/cockroach/tags:

<code>docker pull cockroachdb/cockroach:v21.2.7</code>

Deploy the local charm

<code>
juju deploy ./testcharms/charm-repo/focal/cockroach --resource cockroachdb-image=cockroachdb/cockroach:v21.2.7
</code>

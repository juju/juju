# juju-qa-container-resource

This charm is designed for testing container resources for charms on kubernetes.

The charm starts all pebble services on the container images, sends a GET
request to the container at port 8080, and then sets the response and the charm
status message.

A container image resource is included with this charm. It contains Go server
serving the name of the resource instance at port 8080. It is started by a
pebble service included in the image.
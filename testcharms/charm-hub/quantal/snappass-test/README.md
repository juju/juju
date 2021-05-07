# snappass-test

This is a test charm for Pinterest's [SnapPass](https://github.com/pinterest/snappass) that uses the Python Operator Framework and K8s sidecar containers with [Pebble](https://github.com/canonical/pebble).

You'll need state of the art tools, at 2021-04-15 these come from:

- Charmcraft: [from this branch](https://github.com/facundobatista/charmcraft/tree/ociimages-draft)
- Juju 2.9-rc10 (currently installed from latest/candidate)

To try it locally, also need to setup `microk8s`:

```
sudo snap install juju-wait --classic
sudo snap install microk8s --classic
sudo snap alias microk8s.kubectl kubectl

microk8s.reset  # Warning! Clean slate!
microk8s.enable dns dashboard registry storage
microk8s.status --wait-ready
microk8s.config | juju add-k8s myk8s --client
```

And setup Juju to use it:

```
juju bootstrap myk8s
juju add-model test-staging --config charmhub-url=https://api.staging.charmhub.io
juju status
```

It's a good idea to turn on debug logging:

```
juju model-config logging-config=DEBUG
juju debug-log
```


## Steps to try everything

Set up project:

    $ git clone https://github.com/facundobatista/snappass-test.git
    $ cd snappass-test

...and edit `metadata.yaml` to change charm's name, otherwise will collide with mine.

Build the charm, register the name and upload it:

```
$ charmcraft build
Created 'facundo-snappass-test.charm'.
$ charmcraft register facundo-snappass-test
You are now the publisher of charm 'facundo-snappass-test' in Charmhub.
$ charmcraft upload facundo-snappass-test.charm 
Revision 1 of 'facundo-snappass-test' created
$ charmcraft resources facundo-snappass-test
Charm Rev    Resource        Type       Optional
1            redis-image     oci-image  True
             snappass-image  oci-image  True
```

Upload the resources:

```
$ charmcraft upload-resource facundo-snappass-test snappass-image --image=benhoyt/snappass-test
Revision 1 created of resource 'snappass-image' for charm 'facundo-snappass-test'
$ charmcraft -v upload-resource facundo-snappass-test redis-image --image=redis
Revision 1 created of resource 'redis-image' for charm 'facundo-snappass-test'
```

Release the charm using those resources:

```
$ charmcraft release facundo-snappass-test --revision=1 --channel=stable --resource=redis-image:1 --resource=snappass-image:1
Revision 1 of charm 'facundo-snappass-test' released to stable (attaching resources: 'redis-image' r1, 'snappass-image' r1)
```

Deploy!

```
$ juju deploy facundo-snappass-test
```

LTS Update Issues
=================

When we update ubuntu LTS versions we run into some issues updating tests. When updating from Trusty to Xenial we attempted to simplify future updates. There are still a few places that need modification when an LTS update happends. `grep -r LTS-dependent` at the top of the core repo to find the locations that are expected to require updates at the next LTS release. As of this writing that would be in the following files:

 - cmd/juju/service/bundle_test.go
 - provider/ec2/export_test.go
 - provider/ec2/image_test.go
 - testing/base.go

There will likely be other locations by the next release, but this should provide a headstart.

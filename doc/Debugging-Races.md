<!-- TODO(gfouillet): do not merge into 4.0, or delete whenever merged (reason: related to mongodb) -->

## Triggering races

Someone pointed me to the [stress test script](https://github.com/juju/juju/wiki/Stress-Test) on this wiki. While it was
immeasurably helpful, I have found a few more things that help me reproduce and instrument races. The previous link also
includes an updated script which adds a counter and timing to the output. I find it useful to know how long it took to
trigger a race, or how long it was stressed without triggering.

I also install [stress](http://linux.die.net/man/1/stress) to create contention on cpus, ram, io, etc.     
E.g. ```stress --cpu 8 --io 4 --vm 2 -d 2 --timeout 60m```. I imagine this would be nice to randomize in the script(s)
linked in the first paragraph.

## AWS

I find that running on a small or medium instance on AWS helps to trigger races more quickly than my local hardware.
Particularly useful are instances that shares CPU time -- t._n_ instances currently. If you locally build the test to
stress you may still need to rsync over the build environment as some tests look for files in the build tree. You'll
also need to install mongo.

TODO: more details on stressing on AWS.

* snap install juju-db




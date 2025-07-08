(juju-roadmap-and-releases)=
# Juju Roadmap & Releases

> See also: {ref}`upgrade-your-deployment`

This document is about our releases of Juju, that is, the `juju` CLI client and the Juju agents.

- We release new minor version (the 'x' of m.x.p) approcimately every 3 months.
- Patch releases for supported series are released every month
- Once we release a new major version, the latest minor version of the previous release will become an LTS (Long Term Support) release.

- Minor releases are supported with bug fixes for a period of 6 months from their release date, and a further 3 months of security fixes. LTS releases will receive security fixes for 5 years.

- 4.0 is an exception to the rule, as it is still under development. We plan on releasing beta versions that are content driven and not time.

The rest of this document gives detailed information about each release.


<!--THERE ARE ISSUES WITH THE TARBALL.
```
$ wget https://github.com/juju/juju/archive/refs/tags/juju-2.9.46.zip
$ tar -xf juju-2.9.46.tar.gz
$ cd juju-juju-2.9.46
$ go run version/helper/main.go
3.4-beta1
```
ADD WHEN FIXED.
-->


<!--TEMPLATE
### 🔸 **Juju 2.9.X**  - <DATE>  <--leave this as TBC until released into stable!

🛠️ Fixes:

- Juju 3.2 doesn't accept token login[(LP203943)](https://bugs.launchpad.net/bugs/2030943)

See the full list in the [milestone page](https://launchpad.net/juju/+milestone/2.9.X).
-->


## ⭐ **Juju 2.9**
> Currently in Security Fix Only support
>
>  April 2028: expected end of security fix support

### 🔸 **Juju 2.9.52** - 07 July 2025

🛠️ Fixes:

- Fix [CVE-2025-0928](https://github.com/juju/juju/security/advisories/GHSA-4vc8-wvhw-m5gv)
- Fix [CVE-2025-53512](https://github.com/juju/juju/security/advisories/GHSA-r64v-82fh-xc63)
- Fix [CVE-2025-53513](https://github.com/juju/juju/security/advisories/GHSA-24ch-w38v-xmh8)
- fix: 2.9 pki for go 1.24.4 by @jameinel in https://github.com/juju/juju/pull/19972
- fix(apiserver): avoid splitting untrusted data by @jub0bs in https://github.com/juju/juju/pull/18970
- fix: static-analysis by @jack-w-shaw in https://github.com/juju/juju/pull/19353

See the full list in the [milestone page](https://launchpad.net/juju/+milestone/2.9.52).

### 🔸 **Juju 2.9.51** - 30 August 2024

🛠️ Fixes:

- Fix [CVE-2024-7558](https://github.com/juju/juju/security/advisories/GHSA-mh98-763h-m9v4)
- Fix [CVE-2024-8037](https://github.com/juju/juju/security/advisories/GHSA-8v4w-f4r9-7h6x)
- Fix [CVE-2024-8038](https://github.com/juju/juju/security/advisories/GHSA-xwgj-vpm9-q2rq)

See the full list in the [milestone page](https://launchpad.net/juju/+milestone/2.9.51).


### 🔸 **Juju 2.9.50**  - 25 July 2024

🛠️ Fixes:

- Fix [CVE-2024-6984](https://www.cve.org/CVERecord?id=CVE-2024-6984)

See the full list in the [milestone page](https://launchpad.net/juju/+milestone/2.9.50).


### 🔸 **Juju 2.9.49**  - 8 April 2024

🛠️ Fixes:

- Fix pebble [CVE-2024-3250](https://github.com/canonical/pebble/security/advisories/GHSA-4685-2x5r-65pj)

See the full list in the [milestone page](https://launchpad.net/juju/+milestone/2.9.49).

### 🔸 **Juju 2.9.47** - 18 March 2024

🛠️ Fixes:

- model config num-provision-workers can lockup a controller ([LP2053216](https://bugs.launchpad.net/bugs/2053216))

See the full list in the [milestone page](https://launchpad.net/juju/+milestone/2.9.47).


### 🔸 **Juju 2.9.46** - 5 Dec 2023

🛠️ Fixes:

- juju refresh to revision is ignored w/ charmhub ([LP1988556](https://bugs.launchpad.net/bugs/1988556))
- updated controller api addresses lost when k8s unit process restarts ([LP2037478](https://bugs.launchpad.net/bugs/2037478))
- Juju client is trying to reach index.docker.io when using custom caas-image-repo ([LP2037744](https://bugs.launchpad.net/bugs/2037744))
- juju deploy jammy when focal requested ([LP2039179](https://bugs.launchpad.net/bugs/2039179))

See the full list in the [milestone page](https://launchpad.net/juju/+milestone/2.9.46).

### 🔸 **Juju 2.9.45** - 27 Sep 2023

🛠️ Fixes:

- panic: charm nil pointer dereference ([LP2034707](https://bugs.launchpad.net/juju/+bug/2034707))
- juju storage mounting itself over itself ([LP1830228](https://bugs.launchpad.net/juju/+bug/1830228))
- upgrade-series prepare puts units into failed state if a subordinate does not support the target series ([LP2008509](https://bugs.launchpad.net/juju/+bug/2008509))
- data bags go missing ([LP2011277](https://bugs.launchpad.net/juju/+bug/2011277))

See the full list in the [milestone page](https://launchpad.net/juju/+milestone/2.9.45).

### 🔸 **Juju 2.9.44**  - 20 July 2023

Fixes several major bugs in 2.9.44 **6 High** / 1 Medium

🛠️ Fixes:

- Unit is stuck in unknown/lost status when scaling down [(LP1977582)](https://bugs.launchpad.net/bugs/1977582)
- failed to migrate binaries: charm local:focal/ubuntu-8 unexpectedly assigned local:focal/ubuntu-7 [(LP1983506)](https://bugs.launchpad.net/bugs/1983506)
- Provide way for admins of controllers to remove models from other users [(LP2009648)](https://bugs.launchpad.net/bugs/2009648)
- Juju SSH doesn't attempt to use ED25519 keys [(LP2012208)](https://bugs.launchpad.net/bugs/2012208)
- Some Relations hooks not firing over CMR [(LP2022855)](https://bugs.launchpad.net/bugs/2022855)
- Charm refresh from podspec to sidecar k8s/caas charm leaves agent lost units [(LP2023117)](https://bugs.launchpad.net/bugs/2023117)
- python-libjuju doesn't populate the 'charm' field from subordinates in get_status [(LP1987332)](https://bugs.launchpad.net/bugs/1987332)

See the full list in the [milestone page](https://launchpad.net/juju/+milestone/2.9.44).

### 🔸 **Juju 2.9.43** - 13 June 2023

Fixes several major bugs in 2.9.43 **5 Critical / 10 High**

🛠️ Fixes:

- Containers are killed before any 'on stop/remove' handlers have a chance to run ([LP1951415](https://bugs.launchpad.net/juju/+bug/1951415))
-  the target controller keeps complaining if a sidecar app was migrated due to statefulset apply conflicts in provisioner worker ([LP2008744](https://bugs.launchpad.net/juju/+bug/2008744))
- migrated sidecar unit agents keep restarting due to a mismatch charmModifiedVersion ([LP2009566](https://bugs.launchpad.net/juju/+bug/2009566))

See the full list in the [milestone page](https://launchpad.net/juju/+milestone/2.9.43).

### 🔸 **Juju 2.9.42**  - 7 March 2023

Fixes several major bugs in 2.9.42.

🛠️ Fixes:

- Juju forces specifying series on metadata.yaml ([LP1992833](https://bugs.launchpad.net/juju/+bug/1992833))
- LXD unit binding to incorrect MAAS space with no subnets crashes with error ([LP1994124](https://bugs.launchpad.net/juju/+bug/1994124))
- panic when getting juju full status ([LP2002114](https://bugs.launchpad.net/juju/+bug/2002114))
- max-debug-log-duration: expected string or time.Duration ([LP2003149](https://bugs.launchpad.net/juju/+bug/2003149))
- juju using Openstack provider does not remove security groups ([LP1940637](https://bugs.launchpad.net/juju/+bug/1940637))

See the full list in the [milestone page](https://launchpad.net/juju/+milestone/2.9.42).

### 🔸 **Juju 2.9.38**  - 17 January 2023

This release fixes some critical issues ending in panic and a some problems regarding the usage of lxd 5.x.

The main fixes in this release are below.

🛠️ Fixes:
- Juju panics when trying to add-k8s with no obvious storage to use ([LP#1996808](https://bugs.launchpad.net/bugs/1996808))
- Panic after agent-logfile-max-backups-changed ([LP#2001732](https://bugs.launchpad.net/bugs/2001732))
- Failing to deploy lxd containers with lxd latest/stable as lxd version 5.x is promoted to latest/stable ([LP#2002309](https://bugs.launchpad.net/bugs/2002309))

See the full list in the [milestone page](https://launchpad.net/juju/+milestone/2.9.38).

### 🔸 **Juju 2.9.37** - 15 Nov 2022

The main fixes in this release are below. A startup issue on k8s is fixed, plus an intermittent situation where container creation can fail.

🛠️ Fixes (more on the milestone):

- Provisioner worker pool errors cause on-machine provisioning to cease ([LP#1994488](https://bugs.launchpad.net/bugs/1994488))
- charm container crashes resulting in storage-attach hook error ([LP#1993309](https://bugs.launchpad.net/bugs/1993309))

See the full list in the [milestone page](https://launchpad.net/juju/+milestone/2.9.37).

### 🔸 **Juju 2.9.35** - 12 Oct 2022

🛠️ Fixes (more on the milestone):

- juju series inconsistency deploying by charm vs bundle ([LP1983581](https://bugs.launchpad.net/juju/+bug/1983581))
- Azure provider: New region 'qatarcentral' ([LP1988511](https://bugs.launchpad.net/juju/+bug/1988511))
- Better error message for add-model with no credential ([LP1988565](https://bugs.launchpad.net/juju/+bug/1988565))
- juju ssh does not work for non admin user for a k8s model ([LP1989160](https://bugs.launchpad.net/juju/+bug/1989160))
- refresh: ERROR selecting releases: unknown series for version: "22.10" ([LP1990182](https://bugs.launchpad.net/juju/+bug/1990182))

See the full list in the [milestone page](https://launchpad.net/juju/+milestone/2.9.35).

### 🔸 **Juju 2.9.34** - 7 Sep 2022

🛠️ Fixes (more on the milestone):

- cloudinit-userdata doesn't handle lists in runcmd ([LP1759398](https://bugs.launchpad.net/bugs/1759398))
- juju doesn't remove KVM virtual machines on maas nodes when using `juju remove-unit` ([LP1982960](https://bugs.launchpad.net/bugs/1982960))
- juju does not honor --channel latest/* option ([LP1984061](https://bugs.launchpad.net/bugs/1984061))
- cannot deploy bundle, invalid fields ([LP1984133](https://bugs.launchpad.net/bugs/1984133))
- juju assumes lxd always available on machine nodes ([LP1986877](https://bugs.launchpad.net/bugs/1986877))

See the full list in the [milestone page](https://launchpad.net/juju/+milestone/2.9.34).

### 🔸 **Juju 2.9.33** - 9 Aug 2022

🛠️ Fixes (many more on the milestone):

- lxd profiles not being applied ([LP](https://bugs.launchpad.net/bugs/1982329))
- remove a unit with lxd profile doesn't update ([LP](https://bugs.launchpad.net/bugs/1982599))
- Instance poller reports: states changing too quickly ([LP](https://bugs.launchpad.net/bugs/1948824))
- juju wants to use the LXD UNIX socket when configured to use HTTP ([LP](https://bugs.launchpad.net/bugs/1980811))
- cannot pin charm revision without mention series in bundle ([LP](https://bugs.launchpad.net/bugs/1982921))
- add retry-provisioning --all ([LP](https://bugs.launchpad.net/bugs/1940440))

See the full list in the [milestone page](https://launchpad.net/juju/+milestone/2.9.33).

### 🔸 **Juju 2.9.32** - 24 June 2022

🛠️ Fixes:

- Juju 2.9.31 breaks yaml format accepted by `juju add-credential`([LP](https://bugs.launchpad.net/bugs/1976620))
- azure failed provisioning: conflict with a concurrent request([LP](https://bugs.launchpad.net/bugs/1973829))
- Juju attach-resource returns 'unsupported resource type ""' error([LP](https://bugs.launchpad.net/bugs/1975726))
- OpenStack: open-port icmp doesn't work([LP](https://bugs.launchpad.net/bugs/1970295))
- Juju bootstrap aks can't find storage([LP](https://bugs.launchpad.net/bugs/1976434))

See the full list in the [milestone page](https://launchpad.net/juju/+milestone/2.9.32).

### 🔸 **Juju 2.9.31** - 31 May 2022

🛠️ Fixes:

- juju controller doesn't reference juju-https-proxy when deploying from charmhub ([LP](https://bugs.launchpad.net/bugs/1973738))
- sidecar application caasapplicationprovisioner worker restarts due to status set failed ([LP](https://bugs.launchpad.net/bugs/1975457))
- LXD container fails to start due to UNIQUE constraint on container.name ([LP](https://bugs.launchpad.net/bugs/1945813))
- k8s application stuck in an unremoveable state ([LP](https://bugs.launchpad.net/bugs/1948695))
- Juju keeps creating OpenStack VMs if it cannot allocate a floating IP ([LP](https://bugs.launchpad.net/bugs/1969309))
- Instance type constraint throws "ambiguous constraints" error on GCP ([LP](https://bugs.launchpad.net/bugs/1970462))

See the full list in the [milestone page](https://launchpad.net/juju/+milestone/2.9.31).

### 🔸 **Juju 2.9.29** - 30 Apr 2022

🛠️ Fixes:

- Controller bootstrap fails on local LXD with "Certificate not found"([LP](https://bugs.launchpad.net/bugs/1968849))
- Juju unable to add a k8s 1.24 k8s cloud([LP](https://bugs.launchpad.net/bugs/1969645))
- model migration treats "TryAgain" as a fatal error([LP](https://bugs.launchpad.net/bugs/1968058))
- juju 2.9.26 unable to deploy centos7([LP](https://bugs.launchpad.net/bugs/1964815))

See the full list in the [milestone page](https://launchpad.net/juju/+milestone/2.9.29).

### 🔸 **Juju 2.9.28** - 08 Apr 2022

🛠️ Fixes:

- Juju renders invalid netplan YAML for nameservers in IPv4/IPv6 dual-stack environment ([LP](https://bugs.launchpad.net/bugs/1883701))
- juju 2.9.27 glibc errors([LP](https://bugs.launchpad.net/bugs/1967136))
- Juju controller keeps restarting when deployed with juju-ha-space and juju-mgmt-space ([LP](https://bugs.launchpad.net/bugs/1966983))

See the full list in the [milestone page](https://launchpad.net/juju/+milestone/2.9.28).

### 🔸 **Juju 2.9.27** - 21 Mar 2022

Candidate release:  18 Mar 2022

🛠️ Fixes:

- juju client panics during bootstrap on a k8s cloud ([LP1964533](https://bugs.launchpad.net/bugs/1964533))
- Controller upgrade ends up with locked upgrade ([LP1942447](https://bugs.launchpad.net/bugs/1942447))
- juju fails to upgrade ha controllers on for (at least) lxd controllers ([LP1963924](https://bugs.launchpad.net/bugs/1963924))

See the full list in the [milestone page](https://launchpad.net/juju/+milestone/2.9.27).

### 🔸 **Juju 2.9.26**  - 12 Mar 2022

This release includes a fix for broken upgrades coming from a deployment with cross model relations to multiple offers hosted on an external controller ([LP1964130](https://bugs.launchpad.net/bugs/1964130)).

🛠️ Fixes:

- 2.9.25 Upgrade Fails for Cross-Controller CMRs([LP1964130](https://bugs.launchpad.net/bugs/1964130))
- Unauthorized for K8s API during charm removal([LP1941655](https://bugs.launchpad.net/bugs/1941655))
- CRD creation fails in pod spec charms on juju 2.9.25([LP1962187](https://bugs.launchpad.net/bugs/1962187))
- Juju prompted for a password in the middle of a bundle deploy([LP1960635](https://bugs.launchpad.net/bugs/1960635))
- Unable to set snap-store-assertions on model-config ([LP1961083](https://bugs.launchpad.net/bugs/1961083))
    - Note: This fix changes how to use log labels in model-config, extra single quotes are no longer required: `juju model-config -m controller "logging-config=#charmhub=TRACE"`



See the full list in the [milestone page](https://launchpad.net/juju/+milestone/2.9.26).


### 🔸 **Juju 2.9.25**  - 24 Feb 2022

This release is significant because it transitions to using the juju-db snap from the `4.4/stable` channel (running mongodb 4.4.11 at the time of writing) for newly bootstrapped controllers. NB the juu-db snap is not used if the default series is changed from `focal` to an earlier vrsion.
Existing controllers which are upgraded to this release will not change the mongo currently in use.

🛠️ Fixes:
- Juju trust not working for K8s charm([LP](https://bugs.launchpad.net/bugs/1957619))
- cannot migration nor upgrade without manual intervention for a machine after a container is removed- ([LP1960235 ](https://bugs.launchpad.net/bugs/1960235))
  - On machines exhibiting the above behavior, the agents will show as lost during the upgrade, you must kill the jujud process on the machine.  This allow it to be restarted and continue the upgrade.
  - Also seen on machine's having an LXD container which haven't been removed.
- destroy model fails if there's a relation to offered application ([LP](https://bugs.launchpad.net/bugs/1954948))
- Sidecar charm get stuck if PodSpec charm with same name was deployed previously ([LP](https://bugs.launchpad.net/bugs/1938907))
- 2.9.22 regression: local charm paths resolved wrongly in bundles ([LP](https://bugs.launchpad.net/bugs/1954933))
- juju migrate failing with manual machines, verifying controller instance([LP](https://bugs.launchpad.net/bugs/1902255))
- Offer permissions are not migrated ([LP](https://bugs.launchpad.net/bugs/1957745))
- destroy model fails if there's a relation to offered application([LP](https://bugs.launchpad.net/bugs/1954948))

See the full list in the [milestone page](https://launchpad.net/juju/+milestone/2.9.25).

### 🔸 **Juju 2.9.22**  - 13 Dec 2021

🛠️ Fixes:

- Juju 2.9.9 fails to bootstrap on AWS ([LP](https://bugs.launchpad.net/bugs/1938019))
- controller migration is very hard when dealing with large deployments ([LP](https://bugs.launchpad.net/bugs/1918680))
- models not logging ([LP](https://bugs.launchpad.net/bugs/1930899))
- ceph-osd is showing as fail ([LP](https://bugs.launchpad.net/bugs/1931567))
- Bootstrap with Juju 2.8.11 breaks on LXD 4.0.8 ([LP](https://bugs.launchpad.net/bugs/1949705))
- juju ssh --proxy not working on aws when targeting containers with FAN addresses ([LP](https://bugs.launchpad.net/bugs/1932547))

See the full list in the [milestone page](https://launchpad.net/juju/+milestone/2.9.22).

### 🔸 **Juju 2.9.21**  - 3 Dec 2021

🛠️ Fixes:

- juju enable-ha fails to cluster on 2.9.18 manual machines ([LP](https://bugs.launchpad.net/bugs/1951813))
- juju storage events are missing JUJU_STORAGE_ID ([LP](https://bugs.launchpad.net/bugs/1948228))
- Juju failing to remove unit due to attached storage stuck dying ([LP](https://bugs.launchpad.net/bugs/1950928))
- Juju creates two units for sidecar CAAS application ([LP](https://bugs.launchpad.net/bugs/1952014))

See the full list in the [milestone page](https://launchpad.net/juju/+milestone/2.9.21).

### 🔸 **Juju 2.9.19**  - 23 Nov 2021

🛠️ Fixes:

- controller models with valid credentials becoming suspended ([LP](https://bugs.launchpad.net/bugs/1841880))
- FIP created in incorrect AZ for instance when bootstrapped against OpenStack. ([LP](https://bugs.launchpad.net/bugs/1928979))
- [2.9.16 & 2.9.17] juju trust gets lost if juju config is run on application ([LP](https://bugs.launchpad.net/bugs/1948496))
- mongo 4.4 has a multiline --version ([LP](https://bugs.launchpad.net/bugs/1949582))

See the full list in the [milestone page](https://launchpad.net/juju/+milestone/2.9.19).

### 🔸 **Juju 2.8.13**  - 11 Nov 2021

This release fixes various issues with Juju **2.8**

🛠️ Fixes:

- Juju ~~2.9.9~~ fails to bootstrap on AWS ([LP](https://bugs.launchpad.net/bugs/1938019))
- controller migration is very hard when dealing with large deployments ([LP](https://bugs.launchpad.net/bugs/1918680))
- models not logging ([LP](https://bugs.launchpad.net/bugs/1930899))
- ceph-osd is showing as fail ([LP](https://bugs.launchpad.net/bugs/1931567))
- Bootstrap with Juju 2.8.11 breaks on LXD 4.0.8 ([LP](https://bugs.launchpad.net/bugs/1949705))
- juju ssh --proxy not working on aws when targeting containers with FAN addresses ([LP](https://bugs.launchpad.net/bugs/1932547))

See the full list in the [milestone page](https://launchpad.net/juju/+milestone/2.8.13).

### 🔸 **Juju 2.9.18** - 8 Nov 2021

🛠️ Fixes:
- agent cannot be up on LXD/Fan network on OpenStack OVN/geneve mtu=1442 ([LP1936842](https://bugs.launchpad.net/bugs/1936842))
- no way to declare a k8s charm with metadata v2 that doesn't need a workload container ([LP1928991](https://bugs.launchpad.net/bugs/1928991))
- Method to run an action in a workload container in sidecar charms ([LP1923822](https://bugs.launchpad.net/bugs/1923822) )

See the full list in the [milestone page](https://launchpad.net/juju/+milestone/2.9.18).

### 🔸 **Juju 2.9.17** - 27 Oct 2021

This release introduces [telemetry](https://discourse.charmhub.io/t/telemetry-and-juju/5188) as a configurable option per model.
It also supports [more OCI image registry providers](https://discourse.charmhub.io/t/initial-private-registry-support/5079) for pulling images used for CAAS models.

🛠️ Fixes:
- Leader role not transferred when the inital leader goes offline ([LP](https://bugs.launchpad.net/bugs/1947409))
- if the primary node of an HA config goes down, the controller stops responding ([LP](https://bugs.launchpad.net/bugs/1947179))
- Trust permissions not ready on install hook in sidecar charms ([LP](https://bugs.launchpad.net/bugs/1942792))
- deployed application loses trust after charm upgrade ([LP](https://bugs.launchpad.net/bugs/1940526))

See the full list in the [milestone page](https://launchpad.net/juju/+milestone/2.9.17).

### 🔸 **Juju 2.9.16** - 11 Oct 2021

🛠️ Fixes:

- Unable to deploy workloads to lxd cloud added to k8s controller ([LP](https://bugs.launchpad.net/bugs/1943265))
- memory usage leading to OOMs on controllers
- LXD bootstrap fails with "Executable /snap/bin/juju-db.mongod not found" ([LP](https://bugs.launchpad.net/bugs/1945752))
- Requested image's type 'virtual-machine' doesn't match instance type 'container' ([LP](https://bugs.launchpad.net/bugs/1943088))

See the full list in the [milestone page](https://launchpad.net/juju/+milestone/2.9.16).

### 🔸 **Juju 2.9.15** - 28 Sept 2021

This release improves the robustness of repeated cross model relation setup / teardown.
There's also some improvements to how raft is used internally to manage leases.

🛠️ Fixes:

- ceph mon does not render data to ceph-rados after redployment of ceph-radosgw only ([LP](https://bugs.launchpad.net/bugs/1940983))
- Unable to remove offers when 2 endpoints are offered with the same application ([LP](https://bugs.launchpad.net/bugs/1873472))
- upgrading 2.9.12 to 2.9.13 gets stuck in 'raftlease response timeout' ([LP](https://bugs.launchpad.net/bugs/1943075))
- pod-spec uniter exits on pending action op when remote caas container died ([LP](https://bugs.launchpad.net/bugs/1943776))

See the full list in the [milestone page](https://launchpad.net/juju/+milestone/2.9.15).

### 🔸 **Juju 2.9.14** - 14 Sept 2021

This release fixes an upgrade issue found during testing of the 2.9.13 release.
There's also an additional fix for an earlier regression deploying LXD containers on AWS.

🛠️ Fixes:

- Juju fails to provision LXD containers with LXD >= 4.18 ([LP](https://bugs.launchpad.net/bugs/1942864))
- Juju is unable to match machine address CIDRs to subnet CIDRs on Equinix Metal clouds ([LP](https://bugs.launchpad.net/bugs/1942241))
- Non POSIX-compatible script used in `/etc/profile.d/juju-introspection.sh` ([LP](https://bugs.launchpad.net/bugs/1942430))
- In AWS using spaces and fan network for a private network does not allow LXC containers to start([LP](https://bugs.launchpad.net/bugs/1942950))

See the full list in the [milestone page](https://launchpad.net/juju/+milestone/2.9.14).

### 🔸 **Juju 2.9.13** - Release cancelled, replaced by 2.9.14

This release adds support for pulling images used for CAAS models from private OCI registries! This means you can host your own `jujud-operator`, `charm-base` and `juju-db` images. This initial release focuses on private registries on Dockerhub, with other public cloud registry support coming in a future release. More details in [this post](https://discourse.charmhub.io/t/initial-private-registry-support/5079).

🛠️ Fixes:

- Juju fails to provision LXD containers with LXD >= 4.18 ([LP](https://bugs.launchpad.net/bugs/1942864))
- Juju is unable to match machine address CIDRs to subnet CIDRs on Equinix Metal clouds ([LP](https://bugs.launchpad.net/bugs/1942241))
- Non POSIX-compatible script used in `/etc/profile.d/juju-introspection.sh` ([LP](https://bugs.launchpad.net/bugs/1942430))

### 🔸 **Juju 2.9.12** - 30 Aug 2021

🛠️ Fixes:

- Cross-model relations broken for CAAS ([LP](https://bugs.launchpad.net/bugs/1940298))
- Boot failure when `model-config` sets `snap-proxy` ([LP](https://bugs.launchpad.net/bugs/1940445))
- The `juju export-bundle` command gives error after upgrade ([LP](https://bugs.launchpad.net/bugs/1939601))
- Several updates for the Raft engine that handles leases. These are steps to address ([LP](https://bugs.launchpad.net/juju/+bug/1934524)), though that issue is not completely resolved.

See the full list in the [milestone page](https://launchpad.net/juju/+milestone/2.9.12).

### 🔸 **Juju 2.9.11** - 17 Aug 2021

🛠️ Fixes:

- Resource downloads are very slow in some cases ([LP](https://bugs.launchpad.net/juju/+bug/1905703))
- Upgrading the mongodb snap causes controller to hang without restarting mongod ([LP](https://bugs.launchpad.net/juju/+bug/1922789))
- OpenStack provider: retry-provisioning doesn't work for `Quota exceeded for ...` ([LP](https://bugs.launchpad.net/juju/+bug/1938736))

See the full list in the [milestone page](https://launchpad.net/juju/+milestone/2.9.11).

### 🔸 **Juju 2.9.10** - 03 Aug 2021

A new logging label: `charmhub`. To enable debugging information about Charmhub, you can now use the following:

```
juju model-config -m controller "logging-config='#charmhub=TRACE'"
```

🛠️ Fixes:

- Unable to `upgrade-charm` a pod_spec charm to sidecar charm ([LP](https://bugs.launchpad.net/bugs/1928778))
- OOM and high load upgrading to 2.9.7 ([LP](https://bugs.launchpad.net/bugs/1936684))
- Controller not caching agent binaries across models ([LP](https://bugs.launchpad.net/bugs/1900021))
- Bundle with local metadata v2 k8s sidecar charm fails for "metadata v1" ([LP](https://bugs.launchpad.net/bugs/1936281))
- The `network-get` hook returns the vip as ingress address ([LP](https://bugs.launchpad.net/bugs/1897261))

See the full list in the [milestone page](https://launchpad.net/juju/+milestone/2.9.10).

### 🔸 **Juju 2.9.9** - 19 Jul 2021

🛠️ Fixes:

- Juju 2.9.8 tries to use an empty UID when deleting Kubernetes objects, and cannot remove applications ([LP](https://bugs.launchpad.net/bugs/1936262))
- The `juju-log` output going to machine log file instead of unit log file in Juju 2.9.5 ([LP](https://bugs.launchpad.net/bugs/1933548))
- Deployment of private charms is broken in 2.9 (was working in 2.8) ([LP](https://bugs.launchpad.net/bugs/1932072))
- [Windows] Juju.exe and MicroK8s.exe bootstrap error ([LP](https://bugs.launchpad.net/bugs/1931590))

See the full list in the [milestone page](https://launchpad.net/juju/+milestone/2.9.9).

### 🔸 **Juju 2.9.8** - 13 Jul 2021

This release introduces support for bootstrapping and deploying workloads to **[Equinix](https://www.equinix.com) cloud**. To try out the new provider:

- Run `juju update-public-clouds --client` to ensure that provider API endpoint list is up to date.
- Add a credential for the equinix cloud (`juju add-credential equinix`). You will need to specify your equinix project ID and provide an API key. You can use the equinix [console](https://console.equinix.com) to look up your project ID and generate API tokens.
- Select a metro area and bootstrap a new controller. For example to bootstrap to the Amsterdam data-center you may run the following command: `juju bootstrap equinix/am`.

Caveats:

- Due to substrate limitations, the equinix provider does not implement support for firewalls. As a result, workloads deployed to machines under the same project ID can reach each other even across Juju models.
- Deployed machines are always assigned both a public and a private IP address. This means that any deployed charms are _implicitly exposed_ and proper access control mechanisms need to be implemented to prevent unauthorized access to the deployed workloads.

This release also introduces **logging labels** which will help with the aggregation of logs via a label rather than a namespace.

```
juju model-config "logging-config='#http=TRACE'"
```

The above will turn on HTTP loggers to trace. This is a new UX feature to help with debugging, it's not been full worked through Juju yet and might be subject to change.

🛠️ Fixes:

- Juju fails to deploy mysql-k8s charm with its image resource ([LP](https://bugs.launchpad.net/bugs/1934416))
- Juju 2.9 failing to create ClusterRoleBinding ([LP](https://bugs.launchpad.net/bugs/1934180))
- Juju interprets `caas-image-repo` containing port number incorrectly ([LP](https://bugs.launchpad.net/bugs/1934707))

See the full list in the [milestone page](https://launchpad.net/juju/+milestone/2.9.8).

### 🔸 **Juju 2.9.7** - 06 Jul 2021

🛠️ Fixes:

- Juju upgrade 2.9 controller from local branch fails with wrong namespace. ([LP](https://bugs.launchpad.net/bugs/1930798))
- Unit network data not populated on peer relations in sidecar charms ([LP](https://bugs.launchpad.net/bugs/1922133))
- A `juju refresh --switch ./local` fails for metadata v1 charm ([LP](https://bugs.launchpad.net/bugs/1925670))
- A migrated CaaS model will be left in the cluster after model destroyed ([LP](https://bugs.launchpad.net/bugs/1927656))
- Unable to deploy postgresql-k8s charm from charmhub ([LP](https://bugs.launchpad.net/bugs/1928182))
- Unable to deploy bundle with sidecar and pod_spec charms ([LP](https://bugs.launchpad.net/bugs/1928796))
- IP address sometimes not set or incorrect on pebble_ready event ([LP](https://bugs.launchpad.net/bugs/1929364))
- Improve `juju ssh` on k8s poor ux ([LP](https://bugs.launchpad.net/bugs/1929904))
- Support encrypted EBS volumes for bootstrapping controllers on AWS ([LP](https://bugs.launchpad.net/bugs/1931139))
- Document and support `charmcraft`'s bundle.yaml fields ([LP](https://bugs.launchpad.net/bugs/1931140))
- install hook run after juju upgrade-model 2.7.8 to 2.9.4 ([LP](https://bugs.launchpad.net/bugs/1931708))
- controller fails to bring up `jujud` machine ([LP](https://bugs.launchpad.net/bugs/1871224))
- The `juju ssh --proxy` command is not working on aws when targeting containers with FAN addresses ([LP](https://bugs.launchpad.net/bugs/1932547))
- The `juju resources` revision date format uses year-date-month format instead of year-month-date ([LP](https://bugs.launchpad.net/bugs/1933705))
- Using `juju config` with empty values erroneously resets since 2.9 ([LP](https://bugs.launchpad.net/bugs/1934151))

See the full list in the [milestone page](https://launchpad.net/juju/+milestone/2.9.7).

### 🔸 **Juju 2.9.5**
Release notes [here](https://discourse.charmhub.io/t/juju-2-9-5-release-notes/4750).

### 🔸 **Juju 2.9.4**
Release notes [here](https://discourse.charmhub.io/t/juju-2-9-4-release-notes/4660).

### 🔸 **Juju 2.9.3**
Release notes [here](https://discourse.charmhub.io/t/juju-2-9-3-release-notes/4628).

### 🔸 **Juju 2.9.2**
Release notes [here](https://discourse.charmhub.io/t/juju-2-9-2-release-notes/4605).

### 🔸 **Juju 2.9.0**
Release notes [here](https://discourse.charmhub.io/t/juju-2-9-0-release-notes/4525).


## **Before Juju 2.9 (all EOL)**

### 🔸 **Juju 2.8**


```{caution}

Juju 2.8 series is EOL

```
- [2.8.11](https://discourse.charmhub.io/t/juju-2-8-11-release-notes)
- [2.8.10](https://discourse.charmhub.io/t/juju-2-8-10-release-notes/4374)
- [2.8.9](https://discourse.charmhub.io/t/2-8-9-release-notes/4197/2)
- [2.8.8](https://discourse.charmhub.io/t/juju-2-8-8-release-notes/4128/2)
- [2.8.7](https://discourse.charmhub.io/t/juju-2-8-7-release-notes/3880/2)
- [2.8.6](https://discourse.charmhub.io/t/juju-2-8-6-release-notes/3649)
- [2.8.5](https://discourse.charmhub.io/t/juju-2-8-5-hotfix-release-notes/3638)
- [2.8.4](https://discourse.charmhub.io/t/juju-2-8-4-release-notes/3639)
- [2.8.3](https://discourse.charmhub.io/t/juju-2-8-3-hotfix-release-notes/3570)
- [2.8.2](https://discourse.charmhub.io/t/juju-2-8-2-release-notes/3551)
- [2.8.1](https://discourse.charmhub.io/t/juju-2-8-1-release-notes/3296)
- [2.8.0](https://discourse.charmhub.io/t/juju-2-8-0-release-notes/3180)



### 🔸 **Juju 2.7**


```{caution}

Juju 2.7 series is EOL

```
- [2.7.8](https://discourse.charmhub.io/t/juju-2-7-8-release-notes/3340)
- [2.7.7](https://discourse.charmhub.io/t/juju-2-7-7-release-notes/3293)
- [2.7.6](https://discourse.charmhub.io/t/juju-2-7-6-release-notes/2888)
- [2.7.5](https://discourse.charmhub.io/t/juju-2-7-5-release-notes/2772)
- [2.7.4](https://discourse.charmhub.io/t/juju-2-7-4-release-notes/2787)
- [2.7.3](https://discourse.jujucharms.com/t/juju-2-7-3-release-notes/2702)
- [2.7.2](https://discourse.jujucharms.com/t/juju-2-7-2-release-notes/2667)
- [2.7.1](https://discourse.jujucharms.com/t/juju-2-7-1-release-notes/2495)
- [2.7.0](https://discourse.jujucharms.com/t/juju-2-7-release-notes/2380)


### 🔸 **Juju 2.6**


```{caution}

Juju 2.6 series is EOL

```
- [2.6.10](https://discourse.jujucharms.com/t/juju-2-6-10-release-notes/2285)
- [2.6.9](https://discourse.jujucharms.com/t/juju-2-6-9-release-notes/2100)
- [2.6.8](https://discourse.jujucharms.com/t/juju-2-6-8-release-notes/2000)
- [2.6.6](https://discourse.jujucharms.com/t/juju-2-6-6-release-notes/1890)
- [2.6.5](https://discourse.jujucharms.com/t/juju-2-6-5-release-notes/1630)
- [2.6.4](https://discourse.jujucharms.com/t/juju-2-6-4-release-notes/1583)
- [2.6.3](https://discourse.jujucharms.com/t/juju-2-6-3-release-notes/1541)
- [2.6.2](https://discourse.jujucharms.com/t/juju-2-6-2-release-notes/1474)
- [2.6.1](https://discourse.jujucharms.com/t/juju-2-6-1-release-notes/1473)


### 🔸 **Juju 2.5**


```{caution}

Juju 2.5 series is EOL

```
- [2.5.8](https://discourse.jujucharms.com/t/juju-2-5-8-release-notes/1617)
- [2.5.7](https://discourse.jujucharms.com/t/juju-2-5-7-release-notes/1432)
- [2.5.4](https://discourse.jujucharms.com/t/juju-2-5-4-release-notes/1326)
- [2.5.3](https://discourse.jujucharms.com/t/juju-2-5-3-release-notes/1307)
- [2.5.2](https://discourse.jujucharms.com/t/2-5-2-release-notes/1270)
- [2.5.1](https://discourse.jujucharms.com/t/2-5-1-release-notes/1178)
- [2.5.0](https://discourse.jujucharms.com/t/2-5-0-release-notes/1177)


### 🔸 **Juju 2.4**


```{caution}

Juju 2.4 series is EOL

```

- [2.4.7](https://discourse.jujucharms.com/t/2-4-7-release-notes/1176)
- [2.4.6](https://discourse.jujucharms.com/t/2-4-6-release-notes/1175)
- [2.4.5](https://discourse.jujucharms.com/t/2-4-5-release-notes/1174)
- [2.4.4](https://discourse.jujucharms.com/t/2-4-4-release-notes/1173)
- [2.4.3](https://discourse.jujucharms.com/t/2-4-3-release-notes/1172)
- [2.4.2](https://discourse.jujucharms.com/t/2-4-2-release-notes/1171)
- [2.4.1](https://discourse.jujucharms.com/t/2-4-1-release-notes/1170)
- [2.4.0](https://discourse.jujucharms.com/t/2-4-0-release-notes/1169)

---
myst:
  html_meta:
    description: "Juju 3.x release notes archive: versions before 3.6 that are end-of-life, including Juju 3.5, 3.4, 3.3, 3.2, 3.1, and 3.0."
---

(juju3xx)=
# **Before Juju 3.6 (all EOL)**


## ⭐ **Juju 3.5**
> 30 Apr 2025: end of security fix support
>
> 28 Feb 2025: end of bug fix support

```{caution}

Juju 3.5 series is EOL

```

### 🔸 **Juju 3.5.7**
🗓️ 11 Mar 2025

🛠️ Fixes:
* feat(security): add SECURITY.md for reporting security issues by @anvial in [#18245](https://github.com/juju/juju/pull/18245)
* fix: find azure address prefix from new api result; by @ycliuhw in [#18776](https://github.com/juju/juju/pull/18776)
* fix: add recent introduced aws regions to update public clouds by @CodingCookieRookie in [#18774](https://github.com/juju/juju/pull/18774)
* fix: reflecting watcher in error handling by @hpidcock in [#18791](https://github.com/juju/juju/pull/18791)
* fix: upgrade go version to 1.23.6 to address GO-2025-3447 vuln by @nvinuesa in [#18832](https://github.com/juju/juju/pull/18832)
* fix: use after release by @SimonRichardson in [#18868](https://github.com/juju/juju/pull/18868)
* fix(applicationoffers): handle permission validation correctly by @gfouillet in [#18928](https://github.com/juju/juju/pull/18928)
* fix: replicaset update after removing a primary controller in HA by @nvinuesa in [#18965](https://github.com/juju/juju/pull/18965)
* fix(apiserver): avoid splitting untrusted data by @jub0bs in [#18970](https://github.com/juju/juju/pull/18970)
* fix(shallow-copy-addrs): fix shallow copy before shuffle by @SimoneDutto in [#19017](https://github.com/juju/juju/pull/19017)
* fix: install aws cli and creds for tests needing aws ec2 cli by @wallyworld in [#19072](https://github.com/juju/juju/pull/19072)

### 🔸 **Juju 3.5.6**
🗓️ 11 Jan 2025

🛠️ Fixes:
- Fix [controller restart meant sidecar charm k8s workloads restarts](https://bugs.launchpad.net/bugs/2036594)
- Fix [allocate-public-ip not applied in AWS EC2 provider](https://bugs.launchpad.net/bugs/2080238)
- Fix [Cannot log into controller where model was migrated](https://bugs.launchpad.net/bugs/2084043)
- Fix [Potential race in provisioner while destroying model with machine-with-placemen](https://bugs.launchpad.net/bugs/2084448)
- Fix [Juju cannot enable HA when IPs reside in 100.64.0.0/10](https://bugs.launchpad.net/bugs/2091088)
- Fix [juju register failed with permission denied](https://bugs.launchpad.net/bugs/2073741)
- Fix [Juju bootstrap ignores --bootstrap-base parameter](https://bugs.launchpad.net/bugs/2084364)
- Fix [google vpc firewall rule left around after model/controller destroy](https://bugs.launchpad.net/bugs/2090804)

See the full list in the [milestone page](https://launchpad.net/juju/+milestone/3.5.6).

### 🔸 **Juju 3.5.5**
🗓️ 2 Dec 2024

🛠️ Fixes:
- Fix [Peer relation disappears too early on application removal](https://bugs.launchpad.net/bugs/1998282)
- Fix [Logout doesn't remove the cookie](https://bugs.launchpad.net/bugs/2072473)
- Fix [microk8s juju: cloud skip-tls-verify for MAAS cloud does not work](https://bugs.launchpad.net/bugs/2072653)
- Fix [Manual provider error on re-configuring HA](https://bugs.launchpad.net/bugs/2073986)
- Fix [Superfluous checks hindered upgrade-controller](https://bugs.launchpad.net/bugs/2075304)
- Fix [relation-ids does not include peer relations on app removal](https://bugs.launchpad.net/bugs/2076599)
- Fix [Migrating a model with an external offer breaks the consumer](https://bugs.launchpad.net/bugs/2078672)

See the full list in the [milestone page](https://launchpad.net/juju/+milestone/3.5.5).

### 🔸 **Juju 3.5.4**
🗓️ 11 Sep 2024

🛠️ Fixes:

- Fix [CVE-2024-7558](https://github.com/juju/juju/security/advisories/GHSA-mh98-763h-m9v4)
- Fix [CVE-2024-8037](https://github.com/juju/juju/security/advisories/GHSA-8v4w-f4r9-7h6x)
- Fix [CVE-2024-8038](https://github.com/juju/juju/security/advisories/GHSA-xwgj-vpm9-q2rq)
- Fix using ed25519 ssh keys when juju sshing [LP2012208](https://bugs.launchpad.net/juju/+bug/2012208)
- Plus 1 other bug fixes and 17 fixes from 3.4.6

See the full list in the [milestone page](https://launchpad.net/juju/+milestone/3.5.4).

### 🔸 **Juju 3.5.3**
🗓️ 26 Jul 2024

🛠️ Fixes:

- Fix [CVE-2024-6984](https://www.cve.org/CVERecord?id=CVE-2024-6984)

See the full list in the [milestone page](https://launchpad.net/juju/+milestone/3.5.3).

### 🔸 **Juju 3.5.2**
🗓️ 10 Jul 2024

🛠️ Fixes:
- Runtime error: invalid memory address or nil pointer dereference [LP2064174](https://bugs.launchpad.net/juju/+bug/2064174)
- Pebble (juju 3.5.1) cannot write files to workload containers [LP2067636](https://bugs.launchpad.net/juju/+bug/2067636)
- Machines with base ubuntu@24.04 (Noble) flagged as deprecated, blocking controller upgrade [LP2068671](https://bugs.launchpad.net/juju/+bug/2068671)
- Regular expression error when adding a secret [LP2058012](https://bugs.launchpad.net/juju/+bug/2058012)
- Juju should report open-port failures more visibly (than just controller logs) [LP2009102](https://bugs.launchpad.net/juju/+bug/2009102)
- Lower priority juju status overrides app status when a unit is restarting [LP2038833](https://bugs.launchpad.net/juju/+bug/2038833)

### 🔸 **Juju 3.5.1**
🗓️ 30 May 2024

🛠️ Fixes:
* Fix non-rootless sidecar charms by optionally setting SecurityContext. [#17415](https://github.com/juju/juju/pull/17415) [LP2066517](https://bugs.launchpad.net/juju/+bug/2066517)
* Match by MAC in Netplan for LXD VMs [#17327](https://github.com/juju/juju/pull/17327) [LP2064515](https://bugs.launchpad.net/juju/+bug/2064515)
* Fix `SimpleConnector` to set `UserTag` when no client credentials provided [#17309](https://github.com/juju/juju/pull/17309)

### 🔸 **Juju 3.5.0**
🗓️ 7 May 2024

⚙️ Features:
* Optional rootless workloads in Kubernetes charms [#17070](https://github.com/juju/juju/pull/17070)
* Move from pebble 1.7 to pebble 1.10 for Kubernetes charms

🛠️ Fixes:
* juju.rpc panic running request [LP2060561](https://bugs.launchpad.net/juju/+bug/2060561)


## ⭐ **Juju 3.4**

```{caution}

Juju 3.4 series is EOL

```

### 🔸 **Juju 3.4.6**
🗓️ 11 Sep 2024

🛠️ Fixes:

- Fix [CVE-2024-7558](https://github.com/juju/juju/security/advisories/GHSA-mh98-763h-m9v4)
- Fix [CVE-2024-8037](https://github.com/juju/juju/security/advisories/GHSA-8v4w-f4r9-7h6x)
- Fix [CVE-2024-8038](https://github.com/juju/juju/security/advisories/GHSA-xwgj-vpm9-q2rq)
- Fix broken upgrade on k8s [LP2073301](https://bugs.launchpad.net/bugs/2073301)
- Plus 16 other bug fixes.

NOTE: This is the last bug fix release of 3.4.

See the full list in the [milestone page](https://launchpad.net/juju/+milestone/3.4.6).

### 🔸 **Juju 3.4.5**
🗓️ 26 Jul 2024

🛠️ Fixes:

- Fix [CVE-2024-6984](https://www.cve.org/CVERecord?id=CVE-2024-6984)

See the full list in the [milestone page](https://launchpad.net/juju/+milestone/3.4.5).

### 🔸 **Juju 3.4.4**
🗓️ 1 Jul 2024
⚙️ Features:

- Improve error message for "juju register [LP2060265](https://bugs.launchpad.net/juju/+bug/2060265)

🛠️ Fixes:

- Machines with base ubuntu@24.04 (Noble) flagged as deprecated, blocking controller upgrade [LP2068671](https://bugs.launchpad.net/juju/+bug/2068671)
- apt-get install distro-info noninteractive [LP2011637](https://bugs.launchpad.net/juju/+bug/2011637)
- Hide stale data on relation broken [LP2024583](https://bugs.launchpad.net/juju/+bug/2024583)
- juju not respecting "spaces" constraints [LP2031891](https://bugs.launchpad.net/juju/+bug/2031891)
- Juju add-credential google references outdated documentation [LP2049440](https://bugs.launchpad.net/juju/+bug/2049440)
- manual provider: adding space does not update machines [LP2067617](https://bugs.launchpad.net/juju/+bug/2067617)
- Juju controller panic when using token login with migrated model High [LP2068613](https://bugs.launchpad.net/juju/+bug/2068613)
- sidecar unit bouncing uniter worker causes leadership-tracker worker to stop [LP2068680](https://bugs.launchpad.net/juju/+bug/2068680)
- unit agent lost after model migration [LP2068682](https://bugs.launchpad.net/juju/+bug/2068682)
- Dqlite HA: too many colons in address [LP2069168](https://bugs.launchpad.net/juju/+bug/2069168)
- juju wait-for` panic: runtime error: invalid memory address or nil pointer dereference [LP2040554](https://bugs.launchpad.net/juju/+bug/2040554)
- Juju cannot add machines from 'daily' image stream on Azure [LP2067717](https://bugs.launchpad.net/juju/+bug/2067717)
- running-in-container is no longer on $PATH [LP2056200](https://bugs.launchpad.net/juju/+bug/2056200)

See the full list in the [milestone page](https://launchpad.net/juju/+milestone/3.4.4).

### 🔸 **Juju 3.4.3**
🗓️ 5 Jun 2024

🛠️ Fixes:

- Missing dependency for Juju agent installation on Ubuntu minimal [LP2031590](https://bugs.launchpad.net/juju/+bug/2031590)

See the full list in the [milestone page](https://launchpad.net/juju/+milestone/3.4.3).

### 🔸 **Juju 3.4.2**
🗓️ 6 Apr 2024

🛠️ Fixes:

- Fix pebble [CVE-2024-3250](https://github.com/canonical/pebble/security/advisories/GHSA-4685-2x5r-65pj)
- Fix Consume secrets via CMR fails [LP2060222](https://bugs.launchpad.net/juju/+bug/2060222)

See the full list in the [milestone page](https://launchpad.net/juju/+milestone/3.4.2).

### 🔸 **Juju 3.4.0**
🗓️ 15 Feb 2024

⚙️ Features:
* Pebble notices (https://github.com/juju/juju/pull/16428)
* Internal enhancements, performance improvements and bug fixes

🛠️ Fixes:
* Homogenise VM naming in aws & azure [LP2046546](https://bugs.launchpad.net/juju/+bug/2046546)
* Juju can't bootstrap controller on top of k8s/mk8s [LP2051865](https://bugs.launchpad.net/juju/+bug/2051865)
* chown: invalid user: 'syslog:adm' on Oracle [LP1895407](https://bugs.launchpad.net/juju/+bug/1895407)


## ⭐ **Juju 3.3**
```{caution}

Juju 3.3 series is EOL

```

### 🔸 **Juju 3.3.7**
🗓️ 10 Sep 2024

🛠️ Fixes:

- Fix [CVE-2024-7558](https://github.com/juju/juju/security/advisories/GHSA-mh98-763h-m9v4)
- Fix [CVE-2024-8037](https://github.com/juju/juju/security/advisories/GHSA-8v4w-f4r9-7h6x)
- Fix [CVE-2024-8038](https://github.com/juju/juju/security/advisories/GHSA-xwgj-vpm9-q2rq)

NOTE: This is the last release of 3.3. There will be no more releases.

See the full list in the [milestone page](https://launchpad.net/juju/+milestone/3.3.7).

### 🔸 **Juju 3.3.6**
🗓️ 25 Jul 2024

🛠️ Fixes:

- Fix [CVE-2024-6984](https://www.cve.org/CVERecord?id=CVE-2024-6984)

### 🔸 **Juju 3.3.5**
🗓️ 20 May 2024

Final bug fix release of Juju 3.3 series.

🛠️ Fixes:
* Fix deploy regressions [#17061](https://github.com/juju/juju/pull/17061) [#17079](https://github.com/juju/juju/pull/17079)
* Bump Pebble version to v1.4.2 (require admin access for file pull API) [#17137](https://github.com/juju/juju/pull/17137)
* Avoid panics from using a nil pointer [#17188](https://github.com/juju/juju/pull/17188) [LP2060561](https://bugs.launchpad.net/juju/+bug/2060561)
* Async charm download fix backported [#17229](https://github.com/juju/juju/pull/17229) [LP2060943](https://bugs.launchpad.net/juju/+bug/2060943)
* Do not render empty pod affinity info [#17239](https://github.com/juju/juju/pull/17239) [LP2062934](https://bugs.launchpad.net/juju/+bug/2062934)
* Ensure peer units never have their own consumer labels for the application-owned secrets [#17340](https://github.com/juju/juju/pull/17340) [LP2064772](https://bugs.launchpad.net/juju/+bug/2064772)
* Improve handling of deleted secrets [#17365](https://github.com/juju/juju/pull/17365) [LP2065284](https://bugs.launchpad.net/juju/+bug/2065284)
* Fix nil pointer panic when deploying to existing container [#17366](https://github.com/juju/juju/pull/17366) [LP2064174](https://bugs.launchpad.net/juju/+bug/2064174)
* Don't print a superfluous error when determining platforms of machine scoped placement entities [#17382](https://github.com/juju/juju/pull/17382) [LP2064174](https://bugs.launchpad.net/juju/+bug/2064174)


### 🔸 **Juju 3.3.4**
🗓️ 10 Apr 2024

🛠️ Fixes:

- Fix pebble [CVE-2024-3250](https://github.com/canonical/pebble/security/advisories/GHSA-4685-2x5r-65pj)
- Deploying an application to a specific node fails with invalid model UUID error [LP2056501](https://bugs.launchpad.net/juju/+bug/2056501)
- manual-machines - ERROR juju-ha-space is not set and a unique usable address was not found for machines: 0 [LP1990724](https://bugs.launchpad.net/juju/+bug/1990724)
- juju agent on the controller does not complete after bootstrap [LP2039436](https://bugs.launchpad.net/juju/+bug/2039436)
- ERROR selecting releases: charm or bundle not found for channel "stable", base "amd64/ubuntu/22.04/stable" [LP2054375](https://bugs.launchpad.net/juju/+bug/2054375)
- Non-leader units cannot set a label for app secrets [LP2055244](https://bugs.launchpad.net/juju/+bug/2055244)
- deploy from repository nil pointer error when bindings references a space that does not exist [LP2055868](https://bugs.launchpad.net/juju/+bug/2055868)
- Migrating Kubeflow model from Juju-2.9.46 to Juju-3.4 fails with panic [LP2057695](https://bugs.launchpad.net/juju/+bug/2057695)
- Cross-model relation between 2.9 and 3.3 fails [LP2058763](https://bugs.launchpad.net/juju/+bug/2058763)
- migration between 3.1 and 3.4 fails [LP2058860](https://bugs.launchpad.net/juju/+bug/2058860)
- Offer of non-globally-scoped endpoint should not be allowed [LP2032716](https://bugs.launchpad.net/juju/+bug/2032716)
- `juju config app myconfig=<default value>` "rejects" changes if config was not changed before, but still affects refresh behaviour [LP2043613](https://bugs.launchpad.net/juju/+bug/2043613)
- /sbin/remove-juju-services doesn't cleanup lease table [LP2046186](https://bugs.launchpad.net/juju/+bug/2046186)
- juju credentials stuck as invalid for vsphere cloud [LP2049917](https://bugs.launchpad.net/juju/+bug/2049917)
- Manual provider subnet discovery only happens for new NICs [LP2052598](https://bugs.launchpad.net/juju/+bug/2052598)
- Cannot deploy ceph-proxy charm to LXD container [LP2052667](https://bugs.launchpad.net/juju/+bug/2052667)
- Missing a "dot-minikube" personal-files interface to bootstrap a minikube cloud [LP2051154](https://bugs.launchpad.net/juju/+bug/2051154)

See the full list in the [milestone page](https://launchpad.net/juju/+milestone/3.3.4).

### 🔸 **Juju 3.3.3**
🗓️ 6 Mar 2024
_Note:_ Juju version 3.3.2 was burnt since we discover a showstopper issue during QA, therefore this version will include fixes from 3.3.2.

🛠️ Fixes:
* Bug in controller superuser permission check [LP2053102](https://bugs.launchpad.net/bugs/2053102)
* [3.3.2 candidate] fail to bootstrap controller on microk8s [LP2054930](https://bugs.launchpad.net/bugs/2054930)
* Interrupting machine with running juju-exec tasks causes task to be stuck in running state [LP2012861](https://bugs.launchpad.net/bugs/2012861)
* Juju secret doesn't exist in cross-cloud relation [LP2046484](https://bugs.launchpad.net/bugs/2046484)


See the full list in the [milestone page](https://launchpad.net/juju/+milestone/3.3.3).

### 🔸 **Juju 3.3.1**
🗓️ 25 Jan 2024

🛠️ Fixes:
* Deployed units using Oracle Cloud / OCI provider in wrong region ([LP1864154](https://bugs.launchpad.net/bugs/1864154))
* user created secrets should be migrated after we changed the model's secret backend. ([LP2015967](https://bugs.launchpad.net/bugs/2015967))
* [k8s] topology-key is never set ([LP2040136](https://bugs.launchpad.net/bugs/2040136))
* Machine lock log in multiple places. ([LP2046089](https://bugs.launchpad.net/bugs/2046089))

### 🔸 **Juju 3.3.0**
🗓️ 10 Nov 2023

⚙️ Features:
* User Secrets
* Ignore status when processing controller changes in peergrouper https://github.com/juju/juju/pull/16377
* Allow building with podman using `make OCI_BUILDER=podman ...` https://github.com/juju/juju/pull/16380
* Add support for ARM shapes on Oracle OCI https://github.com/juju/juju/pull/16277
* Remove the last occurences of ComputedSeries https://github.com/juju/juju/pull/16296
* Bump critical packages + add mantic  https://github.com/juju/juju/pull/16426
* Add system identity public key to authorized_keys on new model configs https://github.com/juju/juju/pull/16394
* Export Oracle cloud models with region set from credentials https://github.com/juju/juju/pull/16467
* Missing oracle cloud regions https://github.com/juju/juju/pull/16287


🛠️ Fixes:
* Enable upgrade action. Fix --build-agent juju root finding. https://github.com/juju/juju/pull/16354
* Try and ensure secret access role bindings are created before serving the config to the agent https://github.com/juju/juju/pull/16391
* Fix dqlite binding to ipv6 address. https://github.com/juju/juju/pull/16392
* Filter out icmpv6 when reading back ec2 security groups. https://github.com/juju/juju/pull/16383
* Prevent CAAS Image Path docker request every controller config validation https://github.com/juju/juju/pull/16365
* Fix controller config key finding in md-gen tool. https://github.com/juju/juju/pull/16411
* Fix jwt auth4jaas https://github.com/juju/juju/pull/16431


See the full list in the [milestone page](https://launchpad.net/juju/+milestone/3.2.4), [github full changelog](https://github.com/juju/juju/compare/juju-3.1.6...juju-3.3.0)



## ⭐ **Juju 3.2**
```{caution}

Juju 3.2 series is EOL

```

### 🔸 **Juju 3.2.4**
🗓️ 23 Nov 2023

🛠️ Fixes:

- Juju storage mounting itself over itself ([LP1830228](https://bugs.launchpad.net/juju/+bug/1830228))
- Updated controller api addresses lost when k8s unit process restarts ([LP2037478](https://bugs.launchpad.net/juju/+bug/2037478))
- JWT token auth does not check for everyone@external ([LP2033261](https://bugs.launchpad.net/juju/+bug/2033261))


See the full list in the [milestone page](https://launchpad.net/juju/+milestone/3.2.4), [github full changelog](https://github.com/juju/juju/compare/juju-3.1.6...juju-3.3.0)



### 🔸 **Juju 3.2.3**
🗓️ 13 Sep 2023

🛠️ Fixes:

- Juju 3.2.2 contains pebble with regression ([LP2033094](https://bugs.launchpad.net/juju/+bug/2033094))
- Juju 3.2 doesn't accept token login ([LP2030943](https://bugs.launchpad.net/juju/+bug/2030943))

See the full list in the [milestone page](https://launchpad.net/juju/+milestone/3.2.3).

### 🔸  **Juju 3.2.2** - 21 Aug 2023

Fixes several major bugs in 3.2.2 -- **2 Critical** / 4 High / 2 Medium

🛠️ Fixes:

- juju 3.2 proxy settings not set for lxd/lxc ([LP2025138](https://bugs.launchpad.net/bugs/2025138))
- juju 3.2 admin can't modify model permissions unless it is an admin of the model ([LP2028939](https://bugs.launchpad.net/bugs/2028939))
- Unit is stuck in unknown/lost status when scaling down ([LP1977582](https://bugs.launchpad.net/bugs/1977582))
- Oracle (oci) cloud shapes are hardcoded ([LP1980006](https://bugs.launchpad.net/bugs/1980006))

See the full list in the [milestone page](https://launchpad.net/juju/+milestone/3.2.2).

### 🔸  **Juju 3.2.0** - 26 May 2023


Now secrets can be shared accross models. New support for Lunar Lobster. This new version contains the first piece of code targetting the replacement of Mongo by dqlite. Additional bug fixes and quality of life improvements.

🛠️ Fixes:

- All watcher missing model data ([LP1939341](https://bugs.launchpad.net/bugs/1939341))
- Panic when deploying bundle from file ([LP2017681](https://bugs.launchpad.net/bugs/2017681))
- `add-model` for existing k8s namespace returns strange error message ([LP1994454](https://bugs.launchpad.net/bugs/1994454))
- In AWS, description in security group rules are always empty ([LP2017000](https://bugs.launchpad.net/bugs/2017000))

See the full list in the [milestone page](https://launchpad.net/juju/+milestone/3.2.0).


## ⭐ **Juju 3.1**

```{caution}

Juju 3.1 series is EOL

```

### 🔸 **Juju 3.1.10** - 24 September 2024

🛠️ Fixes:

- Fix [CVE-2024-7558](https://github.com/juju/juju/security/advisories/GHSA-mh98-763h-m9v4)
- Fix [CVE-2024-8037](https://github.com/juju/juju/security/advisories/GHSA-8v4w-f4r9-7h6x)
- Fix [CVE-2024-8038](https://github.com/juju/juju/security/advisories/GHSA-xwgj-vpm9-q2rq)

See the full list in the [milestone page](https://launchpad.net/juju/+milestone/3.1.10).

### 🔸 **Juju 3.1.9**
🗓️ 26 Jul 2024

🛠️ Fixes:

- Fix [CVE-2024-6984](https://www.cve.org/CVERecord?id=CVE-2024-6984)

See the full list in the [milestone page](https://launchpad.net/juju/+milestone/3.1.9).

### 🔸 **Juju 3.1.8**
🗓️ 12 Apr 2024

🛠️ Fixes:

- Fix pebble [CVE-2024-3250](https://github.com/canonical/pebble/security/advisories/GHSA-4685-2x5r-65pj)
- Growth of file descriptors on the juju controller [LP2052634](https://bugs.launchpad.net/juju/+bug/2052634)
- juju agent on the controller does not complete after bootstrap [LP2039436](https://bugs.launchpad.net/juju/+bug/2039436)
- Juju secret doesn't exist in cross-cloud relation [LP2046484](https://bugs.launchpad.net/juju/+bug/2046484)
- Wrong cloud address used in cross model secret on k8s [LP2051109](https://bugs.launchpad.net/juju/+bug/2051109)
- `juju download` doesn't accept --revision although `juju deploy` does [LP1959764](https://bugs.launchpad.net/juju/+bug/1959764)

See the full list in the [milestone page](https://launchpad.net/juju/+milestone/3.1.8).



### 🔸 **Juju 3.1.7** - 3 Jan 2024

🛠️ Fixes **3 Critical / 15 High and more** :

- panic: malformed yaml of manual-cloud causes bootstrap failure ([LP2039322](https://bugs.launchpad.net/bugs/2039322))
- panic: bootstrap failure on vsphere (not repeatable) ([LP2040656](https://bugs.launchpad.net/bugs/2040656))
- Fix panic in wait-for when not using strict equality ([LP2044405](https://bugs.launchpad.net/bugs/2044405))

See the full list in the [milestone page](https://launchpad.net/juju/+milestone/3.1.6).

### 🔸 **Juju 3.1.6** - 5 Oct 2023

🛠️ Fixes **1 Critical / 14 High and more** :

- Juju refresh from ch -> local charm fails with: unknown option "trust" ([LP2034707](https://bugs.launchpad.net/bugs/2017157))
- juju storage mounting itself over itself ([LP1830228](https://bugs.launchpad.net/bugs/1830228))
- Refreshing a local charm reset the "trust" ([LP2019924](https://bugs.launchpad.net/bugs/2019924))
- Juju emits secret-remove hook on tracking secret revision ([LP2023364](https://bugs.launchpad.net/bugs/2023364))
- `juju show-task ""` panics ([LP2024783](https://bugs.launchpad.net/bugs/2024783))

See the full list in the [milestone page](https://launchpad.net/juju/+milestone/3.1.6).

### 🔸 **Juju 3.1.5** - 27 June 2023

Fixes several major bugs in 3.1.5 **1 Critical / 6 High**

🛠️ Fixes:

- Migrating from 2.9 to 3.1 fails ([LP2023756](https://bugs.launchpad.net/bugs/2023756))
- Bootstrap on LXD panics if server is unreachable ([LP2024376](https://bugs.launchpad.net/bugs/2024376))
- Juju should validate the secret backend credential when we change the model-config secret-backend ([LP2015965](https://bugs.launchpad.net/bugs/2015965))
- Juju does not support setting owner label using secret-get ([LP2017042](https://bugs.launchpad.net/bugs/2017042))
- leader remove app owned secret ([LP2019180](https://bugs.launchpad.net/bugs/2019180))
- JUJU_SECRET_REVISION not set in secret-expired hook ([LP2023120](https://bugs.launchpad.net/bugs/2023120))
- Cannot apply model-defaults in isomorphic manner ([LP2023296](https://bugs.launchpad.net/bugs/2023296))

See the full list in the [milestone page](https://launchpad.net/juju/+milestone/3.1.5).

### 🔸 **Juju 3.1.2**
🗓️ 10 Apr 2023

Fixes several major bugs in 3.1.2. **4 Critical / 14 High**

🛠️ Fixes:

- target controller complains if a sidecar app was migrated due to statefulset apply conflicts ([LP2008744](https://bugs.launchpad.net/bugs/2008744))
- migrated sidecar units continue to talk to an old controller after migrate ([LP2008756](https://bugs.launchpad.net/bugs/2008756))
- migrated sidecar units keep restarting ([LP2009566](https://bugs.launchpad.net/bugs/2009566))
- Bootstrap on LXD panics for IP:port endpoint ([LP2013049](https://bugs.launchpad.net/bugs/2013049))

See the full list in the [milestone page](https://launchpad.net/juju/+milestone/3.1.2).

### 🔸 **Juju 3.1.0** - 6 February 2023

Juju 3.1 includes quality of life improvements, removal of charmstore support, introduction of secret backends (Vault and Kubernetes), [open-port support for Kubernetes sidecar charms](https://github.com/juju/juju/pull/14975), introduction of --base CLI argument, [support for multi-homing on OpenStack](https://github.com/juju/juju/pull/14848) and [Bootstrap to LXD VM](https://github.com/juju/juju/pull/15004).

Bug fixes include:

- juju using Openstack provider does not remove security groups on remove-machine after a failed provisioning ([LP1940637](https://bugs.launchpad.net/juju/+bug/1940637))
- k8s: unable to fetch OCI resources - empty id is not valid ([LP1999060](https://bugs.launchpad.net/juju/+bug/1999060))
- Juju doesn't mount storage after lxd container restart ([LP1999758](https://bugs.launchpad.net/juju/+bug/1999758))



## ⭐ **Juju 3.0**

```{caution}

Juju 3.0 series is EOL

```


### 🔸  **Juju 3.0.3** - 15 Feb 2023

This is primarily a bug fix release.

🛠️ Fixes:

- Charm upgrade series hook uses base instead of series ([LP2003858](https://bugs.launchpad.net/bugs/2003858))
- Can't switch from edge channel to stable channel ([LP1988587](https://bugs.launchpad.net/bugs/1988587))
- juju upgrade-model should upgrade to latest, not next major version ([LP1915419](https://bugs.launchpad.net/bugs/1915419))
- unable to retrieve a new secret in same execution hook ([LP1998102](https://bugs.launchpad.net/bugs/1998102))
- Juju doesn't mount storage after lxd container restart ([LP1999758](https://bugs.launchpad.net/bugs/1999758))
- units should be able to use owner label to get the application owned secret ([LP1997289](https://bugs.launchpad.net/bugs/1997289))

See the full list in the [milestone page](https://launchpad.net/juju/+milestone/3.0.3).


### 🔸  **Juju 3.0.2** - 1 Dec 2022


The main fixes in this release are below. Two bootstrap issues are fix: one on k8s and the other on arm64, plus an intermittent situation where container creation can fail. There's also a dashboard fix.

🛠️ Fixes (more on the milestone):

- Provisioner worker pool errors cause on-machine provisioning to cease ([LP#1994488](https://bugs.launchpad.net/bugs/1994488))
- charm container crashes resulting in storage-attach hook error ([LP#1993309](https://bugs.launchpad.net/bugs/1993309))
- not able to bootstrap juju on arm64 with juju 3.0 ([LP#1994173](https://bugs.launchpad.net/bugs/1994173))

See the full list in the [milestone page](https://launchpad.net/juju/+milestone/3.0.2).

### 🔸  **Juju 3.0.0** - 22 Oct 2022

#### What's Changed


##### CLI Changes

**Commands that have been added:**

```text
juju list-operations
juju list-secrets
juju operations
juju secrets
juju show-operation
juju show-secret
juju show-task
juju wait-for
```

**Commands that have been renamed:**

```text
juju constraints (replaces get-constraints)
juju integrate (replaces add-relation, relate)
juju model-constraints (replaces get-model-constraints)
juju set-application-base (replaces set-series)
juju upgrade-machine (replaces upgrade-series)
juju sync-agent-binary (replaces sync-tools)
juju refresh (replaces upgrade-charm)
juju exec (replaces juju run)
juju run (replaces juju run-action)
```

**Commands that have been dropped:**

```text
juju add-subnet
juju attach
juju budget
juju cached-images
juju cancel-action
juju charm
juju create-wallet
juju gui
juju hook-tool
juju hook-tools
juju list-cached-images
juju list-plans
juju list-wallets
juju plans
juju remove-cached-images
juju run-action
juju set-plan
juju set-wallet
juju show-action-output
juju show-action-status
juju show-status
juju show-wallet
juju sla
juju upgrade-dashboard
juju upgrade-gui
juju wallets
```

##### Removal of Juju GUI

Juju GUI is no longer deployed and the --no-gui flag was dropped from juju bootstrap.
The Juju Dashboard replaces the GUI and is deployed using the juju-dashboard charm.


##### Windows charms no longer supported
Windows charms are no longer supported.

##### Bionic and earlier workloads no longer supported
Only workloads on focal and later are supported.

##### No longer create default model on bootstrap
Running juju bootstrap no longer creates a default model. After bootstrap you can use add-model to create a new model to host your workloads.

##### add-k8s helpers for aks, gke, eks
The Juju add-k8s command no longer supports the options "--aks", "--eks", "--gke" for interactive k8s cloud registration. The strict snap cannot execute the external binaries needed to enable this functionality. The options may be added back in a future update.

Note: it's still possible to register AKS, GKE, or EKS clusters by passing the relevant kube config to add-k8s directly.


##### Deprecated traditional kubernetes charms
Traditional kubernetes charms using the pod-spec charm style are deprecated in favor of newer sidecar kubernetes charms.

From juju 3.0, pod-spec charms are pinned to Ubuntu 20.04 (focal) as the base until their removal in a future major version of juju.


##### Rackspace and Cloudsigma providers no longer supported
Rackspace and Cloudsigma providers are no longer supported

#### What's New

##### Juju Dashboard replaces Juju GUI
The Juju Dashboard replaces the GUI; it is deployed via the juju-dashboard charm, which needs to be integrated with the controller application in the controller model.

```
juju bootstrap
juju switch controller
juju deploy juju-dashboard
juju integrate controller juju-dashboard
juju expose juju-dashboard
```

After the juju-dashboard application shows as active, run the dashboard command:

`juju dashboard`

**Note:** the error message which appears if the dashboard is not yet ready needs to be fixed.
([https://bugs.launchpad.net/juju/+bug/1994953](https://bugs.launchpad.net/juju/+bug/1994953))


##### Actions
The client side actions UX has been significantly revamped. See the doc here:
[https://juju.is/docs/olm/manage-actions](https://juju.is/docs/olm/manage-actions)

To understand the changes coming from 2.9 or earlier, see the post here:
[https://discourse.charmhub.io/t/juju-actions-opt-in-to-new-behaviour-from-juju-2-8/2255](https://discourse.charmhub.io/t/juju-actions-opt-in-to-new-behaviour-from-juju-2-8/2255)


##### Secrets

It is now possible for charms to create and share secrets across relation data. This avoids the need for sensitive content to be exposed in plain text. The feature is most relevant to charm authors rather than end users, since how charms use secrets is an internal implementation detail for how workloads are configured and managed. Nonetheless, end users can inspect secrets created by deployed charms:

[https://juju.is/docs/olm/secret](https://juju.is/docs/olm/secret)

[https://juju.is/docs/olm/manage-secrets](https://juju.is/docs/olm/manage-secrets)

Charm authors can learn how to use secrets in their charms:

 [https://juju.is/docs/sdk/add-a-secret-to-a-charm](https://juju.is/docs/sdk/add-a-secret-to-a-charm)

[ https://juju.is/docs/sdk/secret-events](https://juju.is/docs/sdk/secret-events)


##### Juju controller application
The controller model has a Juju controller application deployed at bootstrap. This application currently provides integration endpoints for the Juju dashboard charm. Future work will support integration with the COS stack and others.


##### MongoDB server-side transactions now default
Since the move to mongo 4.4 in juju 2.9, juju now uses server-side transactions.

#### Fixes 🛠️

- deploy k8s charms to juju 3.0 beta is broken ([LP1947105](https://bugs.launchpad.net/bugs/1947105))
- Juju bootstrap failing with various Kubernetes ([LP1905320](https://bugs.launchpad.net/bugs/1905320))
- bootstrapping juju installs 'core' but 'juju-db' depends on 'core18' ([LP1920033](https://bugs.launchpad.net/bugs/1920033))
- bootstrap OCI cloud fails, cannot find image. ([LP1940122](https://bugs.launchpad.net/bugs/1940122))
- Instance key stability in refresh requests ([LP1944582](https://bugs.launchpad.net/bugs/1944582))

See the full list in the [milestone page](https://launchpad.net/juju/+milestone/3.0.0).



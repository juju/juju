(juju-roadmap-and-releases)=
# Juju Roadmap & Releases

> See also: {ref}`upgrade-your-deployment`

This document is about our releases of Juju, that is, the `juju` CLI client and the Juju agents.

- We release new minor version (the 'x' of m.x.p) approximately every 3 months.
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
### 🔸 **Juju 3.6.X**
🗓️ <DATE>  <--leave this as TBC until released into stable!

⚙️ Features:
* feat(secrets): handle NotFound errors in secret backend during `RemoveUserSecrets` by @ca-scribner in [#19169](https://github.com/juju/juju/pull/19169)
* feat: open firewall ports for SSH server  proxy by @kian99 in [#19180](https://github.com/juju/juju/pull/19180)

🛠️ Fixes:
* fix: data race in state pool by @SimonRichardson in https://github.com/juju/juju/pull/19816
* fix: associate DNS config with interfaces as appropriate by @manadart in https://github.com/juju/juju/pull/19890

🥳 New Contributors:
* @nsklikas made their first contribution in https://github.com/juju/juju/pull/19821
-->



## ⭐ **Juju 3.6**
> 30 May 2030: expected end of security fix support
>
> 1 May 2026: expected end of bug fix support

```{note}
Juju 3.6 series is LTS
```

### 🔸 **Juju 3.6.9**
🗓️ 20 Aug 2025

⚙️ Features:
#### New cloud regions
* feat: add aws and azure regions by @adisazhar123 in https://github.com/juju/juju/pull/20087

#### Increased secret content size
The secret content size limit has been increased from 8KiB to 1MiB.
* feat(secrets): increase the allowed size for secret content to 1MiB by @wallyworld in https://github.com/juju/juju/pull/20287

#### Other features
* feat: support import-filesystem for k8s by @jneo8 in https://github.com/juju/juju/pull/19904
* feat: allow migration minion worker to follow redirects by @kian99 in https://github.com/juju/juju/pull/20133
* feat: add tags to Openstack security groups by @adisazhar123 in https://github.com/juju/juju/pull/20169
* feat: allow edge snaps to be used as official builds by @wallyworld in https://github.com/juju/juju/pull/20202

🛠️ Fixes:
#### Openstack
The Openstack Neutron API endpoint was incurring excessive calls due to an inefficient query strategy.<br>
SEV flavors are deprioritised when using constraints to choose a flavor as they are not yet modelled.
* fix: inefficient security group client side filtering by @adisazhar123 in https://github.com/juju/juju/pull/19954
* fix: choose non SEV flavor for Openstack by @adisazhar123 in https://github.com/juju/juju/pull/20299

#### Azure
* fix: azure prem storage pending indefinitely by @CodingCookieRookie in https://github.com/juju/juju/pull/20122

#### LXD
The LXD provider now supports zone constraints.<br>
There are also storage fixes for deploying a charm with multiple storage requirements.
* fix: ensure zone constraints are used with lxd by @wallyworld in https://github.com/juju/juju/pull/20271
* fix: missing availability zones for lxd machines by @adisazhar123 in https://github.com/juju/juju/pull/20339
* fix: sort lxd storage by path before attaching by @wallyworld in https://github.com/juju/juju/pull/20320
* fix: make adding a disk to a lxd container idempotent by @wallyworld in https://github.com/juju/juju/pull/20269

#### Kubernetes
The memory request and limit has been reduced for the charm container and no longer uses the same (possibly large) value
that may have been required for the workload.<br>
The default image repository is now ghcr rather than docker.
* fix: reduce charm memory constraints and fill workload container requests by @CodingCookieRookie in https://github.com/juju/juju/pull/20014

#### Storage
A long occurring intermittent storage bug was fixed where sometimes storage would not be registred as attached and
charms would hang and not run the storage attached hook.
* fix: ensure filesystem attachment watcher sends all events by @wallyworld in https://github.com/juju/juju/pull/20338

#### FAN networking
If the container networking method is set to "local" or "provider", do not set up FAN networking.
* fix: do not detect fan for local or provider container networking by @wallyworld in https://github.com/juju/juju/pull/20353

#### Mitigate possible connection leak
The worker to monitor and update external controller API addreses for cross model relations could needlessly and
constantly bounce due to incorrect detection of address changes. This would cause HTTP connections to churn, possibly
contributing to observed connection / file handle leaks.
* fix: handle script runner errors and don't ignore them by @wallyworld in https://github.com/juju/juju/pull/20352
* fix: do not update external controller info unless needed by @wallyworld in https://github.com/juju/juju/pull/20398

#### Other fixes
* fix: don't flush model when we have no machines by @adisazhar123 in https://github.com/juju/juju/pull/20029
* fix: machine loopback addresses not being accounted by @sombrafam in https://github.com/juju/juju/pull/19998
* fix: use correct version when bootstrapping from edge snap by @wallyworld in https://github.com/juju/juju/pull/20254
* fix: only include resource ID in error message when applying changes by @wallyworld in https://github.com/juju/juju/pull/20295
* fix: k8s model and workload container image updated to repository of target controller during model migration and upgrade by @CodingCookieRookie in https://github.com/juju/juju/pull/20267
* fix: add k8s do not follow path priority for k8s config file by @CodingCookieRookie in https://github.com/juju/juju/pull/20307
* fix: fallback to lexicographical sort if natural sort fails by @adisazhar123 in https://github.com/juju/juju/pull/20313
* fix: life worker reports wrong value by @SimonRichardson in https://github.com/juju/juju/pull/20335

🥳 New Contributors:
* @st3v3nmw made their first contribution in https://github.com/juju/juju/pull/19898
* @jneo8 made their first contribution in https://github.com/juju/juju/pull/19904
* @MattiaSarti made their first contribution in https://github.com/juju/juju/pull/20324


### 🔸 **Juju 3.6.8**
🗓️ 7 Jul 2025

🛠️ Fixes:
* Fix [CVE-2025-0928](https://github.com/juju/juju/security/advisories/GHSA-4vc8-wvhw-m5gv)
* Fix [CVE-2025-53512](https://github.com/juju/juju/security/advisories/GHSA-r64v-82fh-xc63)
* Fix [CVE-2025-53513](https://github.com/juju/juju/security/advisories/GHSA-24ch-w38v-xmh8)
* fix: static-analysis by @jack-w-shaw in https://github.com/juju/juju/pull/19353
* fix: associate DNS config with interfaces as appropriate by @manadart in https://github.com/juju/juju/pull/19890
* fix: solve a model destroy issue on k8s by @wallyworld in https://github.com/juju/juju/pull/19923
* fix: include architecture and base in machine/unit metrics by @jameinel in https://github.com/juju/juju/pull/19930
* fix: 2.9 pki for go 1.24.4 by @jameinel in https://github.com/juju/juju/pull/19972
* fix: speed up status with lots of subordinates by @jameinel in https://github.com/juju/juju/pull/19964
* fix: avoid rereading controller config for every Charm by @jameinel in https://github.com/juju/juju/pull/19963
* fix: add status caching from 2.9 into 3.6 by @jameinel in https://github.com/juju/juju/pull/20012
* fix: set controller UUID in environ by @adisazhar123 in https://github.com/juju/juju/pull/19973

⚙️ Features:
* feat: token auth for migrations by @kian99 in https://github.com/juju/juju/pull/19935

🗒️ Docs:
* docs: remove bundle phase out caveat by @tmihoc in https://github.com/juju/juju/pull/19838
* docs: combine manage deployment docs into a single doc by @tmihoc in https://github.com/juju/juju/pull/19608

### 🔸 **Juju 3.6.7**
🗓️ 9 Jun 2025

🛠️ Fixes:
* fix: use pebble v1.19.1 by @jameinel in https://github.com/juju/juju/pull/19791
* fix: data race in state pool by @SimonRichardson in https://github.com/juju/juju/pull/19816
* fix: charm-user path in docs by @nsklikas in https://github.com/juju/juju/pull/19821
* fix: slice access without guard causes panic by @SimonRichardson in https://github.com/juju/juju/pull/19820
* fix: associate DNS config with interfaces as appropriate by @manadart in https://github.com/juju/juju/pull/19890

🥳 New Contributors:
* @nsklikas made their first contribution in https://github.com/juju/juju/pull/19821

### 🔸 **Juju 3.6.6**
🗓️ 29 May 2025
⚙️ Features:
* feat(secrets): handle NotFound errors in secret backend during `RemoveUserSecrets` by @ca-scribner in [#19169](https://github.com/juju/juju/pull/19169)
* feat: open firewall ports for SSH server  proxy by @kian99 in [#19180](https://github.com/juju/juju/pull/19180)
* feat(ssh): public key authentication for ssh server by @SimoneDutto in [#18974](https://github.com/juju/juju/pull/18974)
* feat: sshtunneler package by @kian99 in [#19285](https://github.com/juju/juju/pull/19285)
* feat: transaction op logging by @manadart in [#19762](https://github.com/juju/juju/pull/19762)

🛠️ Fixes:
* fix: always create K8s unit virtual host key by @kian99 in [#19503](https://github.com/juju/juju/pull/19503)
* fix: model defaults validation by @manadart in [#19462](https://github.com/juju/juju/pull/19462)
* fix: detailed health errors for probe by @jameinel in [#19670](https://github.com/juju/juju/pull/19670)
* fix: broken enable-ha on azure due to a panic caused by a nil pointer  by @wallyworld in [#19695](https://github.com/juju/juju/pull/19695)
* fix: ssh-tunneler worker failure on k8s provider by @kian99 in [#19729](https://github.com/juju/juju/pull/19729)
* fix: warn on dropped error by @MggMuggins in [#19532](https://github.com/juju/juju/pull/19532)

🥳 New Contributors:
* @matthew-hagemann made their first contribution in [#19436](https://github.com/juju/juju/pull/19436)
* @abbiesims made their first contribution in [#19575](https://github.com/juju/juju/pull/19575)
* @MggMuggins made their first contribution in [#19532](https://github.com/juju/juju/pull/19532)

### 🔸 **Juju 3.6.5**
🗓️ 14 Apr 2025
⚙️ Features:
* feat(ssh-server-worker): add feature flag for ssh jump server by @SimoneDutto in [#19364](https://github.com/juju/juju/pull/19364)
* feat: add facade to resolve virtual hostname by @SimoneDutto in [#18995](https://github.com/juju/juju/pull/18995)
* feat: retrieve unit host keys by @ale8k in [#18973](https://github.com/juju/juju/pull/18973)
* feat(state): add state method for ssh connection requests by @SimoneDutto in [#19212](https://github.com/juju/juju/pull/19212)
* feat(state): add cleanup for expired ssh connection requests by @SimoneDutto in [#19239](https://github.com/juju/juju/pull/19239)
* feat(sshworker): add max concurrent connections to ssh server by @SimoneDutto in [#19236](https://github.com/juju/juju/pull/19236)
* feat(ssh-conn-req-facades): add controller and client facade to interact with ssh conn requests by @SimoneDutto in [#19301](https://github.com/juju/juju/pull/19301)
* feat(ssh-server-worker): set unit hostkey for target host by @SimoneDutto in [#19299](https://github.com/juju/juju/pull/19299)

🛠️ Fixes:
* fix(apiserver): avoid splitting untrusted data by @jub0bs in [#18971](https://github.com/juju/juju/pull/18971)
* fix(charmhub): resolve misleading output for info by @leyao-daily in [#19084](https://github.com/juju/juju/pull/19084)
* fix: login to jaas controller by @kian99 in [#19136](https://github.com/juju/juju/pull/19136)
* fix: avoid restart loop of ssh server worker by @kian99 in [#19152](https://github.com/juju/juju/pull/19152)
* fix(bootstrap): support instance-role when bootstrapping by @xtrusia in [#19204](https://github.com/juju/juju/pull/19204)
* fix: facade restriction for "sshserver" facade by @ale8k in [#19220](https://github.com/juju/juju/pull/19220)
* fix(applicationoffer): fix authorization check for list/show offers by @alesstimec in [#19287](https://github.com/juju/juju/pull/19287)
* fix: split model migration status message by @SimonRichardson in [#19255](https://github.com/juju/juju/pull/19255)
* fix: update to use ctrl state & return public key in ssh wire format base64 std encoded by @ale8k in [#19324](https://github.com/juju/juju/pull/19324)
* fix: prevent retry of a successful phase by @SimonRichardson in [#19257](https://github.com/juju/juju/pull/19257)
* fix: close possible leak in ext controller worker by @wallyworld in [#19311](https://github.com/juju/juju/pull/19311)
* fix: revert pull request #19287  by @SimoneDutto in [#19395](https://github.com/juju/juju/pull/19395)
* fix: k8s cloud reuse across controllers by @hpidcock in [#19298](https://github.com/juju/juju/pull/19298)

🥳 New Contributors:
* @sinanawad made their first contribution in [#19179](https://github.com/juju/juju/pull/19179)
* @ahmad-can made their first contribution in [#18784](https://github.com/juju/juju/pull/18784)
* @pamudithaA made their first contribution in [#19155](https://github.com/juju/juju/pull/19155)
* @vlad-apostol made their first contribution in [#19261](https://github.com/juju/juju/pull/19261)
* @alexdlukens made their first contribution in [#19390](https://github.com/juju/juju/pull/19390)

### 🔸 **Juju 3.6.4**
🗓️ 11 Mar 2025
⚙️ Features:
* feat(security): add SECURITY.md for reporting security issues by @anvial in [#18245](https://github.com/juju/juju/pull/18245)
* feat(charmhub): add revision support for info command by @leyao-daily in [#18676](https://github.com/juju/juju/pull/18676)
* feat: add virtual host keys to state by @kian99 in [#18829](https://github.com/juju/juju/pull/18829)
* feat: add support for trust token based authentication on remote LXD  by @nvinuesa in [#18626](https://github.com/juju/juju/pull/18626)
* feat: virtual host keys upgrade step by @kian99 in [#18941](https://github.com/juju/juju/pull/18941)
* feat: ssh server facade and plug in by @ale8k in [#19019](https://github.com/juju/juju/pull/19019)

🛠️ Fixes:
* fix: replicaset update after removing a primary controller in HA by @nvinuesa in [#18965](https://github.com/juju/juju/pull/18965)
* fix: container resource export by @Aflynn50 in [#18898](https://github.com/juju/juju/pull/18898)
* fix(state/charm.go): fix for AddCharmMetadata buildTxn by @alesstimec in [#18990](https://github.com/juju/juju/pull/18990)
* fix(apiserver): avoid splitting untrusted data by @jub0bs in [#18970](https://github.com/juju/juju/pull/18970)
* fix(shallow-copy-addrs): fix shallow copy before shuffle by @SimoneDutto in [#19017](https://github.com/juju/juju/pull/19017)
* fix: avoid error when change for a Pebble notice has been pruned by @benhoyt in [#18981](https://github.com/juju/juju/pull/18981)
* fix: get model info authorization by @alesstimec in [#18959](https://github.com/juju/juju/pull/18959)
* fix: change jaas snap mount path by @kian99 in [#19062](https://github.com/juju/juju/pull/19062)
* fix: install aws cli and creds for tests needing aws ec2 cli by @wallyworld in [#19072](https://github.com/juju/juju/pull/19072)
* fix: login after logout with OIDC by @kian99 in [#19079](https://github.com/juju/juju/pull/19079)
* fix: worker leaking in TestManfioldStart of the SSH server worker by @ale8k in [#19102](https://github.com/juju/juju/pull/19102)

🥳 New Contributors:
* @network-charles made their first contribution in [#19063](https://github.com/juju/juju/pull/19063)
* @andogq made their first contribution in [#19023](https://github.com/juju/juju/pull/19023)


### 🔸 **Juju 3.6.3**
🗓️ 27 Feb 2025
⚙️ Features:
* feat(secrets): add support for using besoke k8s secret backends by @wallyworld in [#18599](https://github.com/juju/juju/pull/18599)
* feat(secrets): add token refresh support to k8s secret backend by @wallyworld in [#18639](https://github.com/juju/juju/pull/18639)
* chore: bump Pebble version to v1.18.0 by @james-garner-canonical in [#18752](https://github.com/juju/juju/pull/18752)
* feat: log MAAS device removals by @manadart in [#18705](https://github.com/juju/juju/pull/18705)
* feat: debug log when we can not find an image by @SimonRichardson in [#18666](https://github.com/juju/juju/pull/18666)
* feat(config): ssh server configuration options by @ale8k in [#18701](https://github.com/juju/juju/pull/18701)
* feat: add hostname parsing by @kian99 in [#18821](https://github.com/juju/juju/pull/18821)
* feat(sshserver worker): adds a base skeleton ssh server worker by @ale8k in [#18627](https://github.com/juju/juju/pull/18627)

🛠️ Fixes:
* fix: juju debug-log --replay and --no-tail by @CodingCookieRookie in [#18601](https://github.com/juju/juju/pull/18601)
* fix: dangling state trackers by @SimonRichardson in [#18611](https://github.com/juju/juju/pull/18611)
* fix: close state pool item on release by @SimonRichardson in [#18614](https://github.com/juju/juju/pull/18614)
* fix(bootstrap): fix bootstrap mirror bug on noble by @jack-w-shaw in [#18659](https://github.com/juju/juju/pull/18659)
* fix: remove server side constraints by @CodingCookieRookie in [#18674](https://github.com/juju/juju/pull/18674)
* fix: support older agents with new k8s secet backend config by @wallyworld in [#18623](https://github.com/juju/juju/pull/18623)
* fix: google model destruction when missing model firewall by @hpidcock in [#18536](https://github.com/juju/juju/pull/18536)
* fix: change String method of intValue to display value not pointer by @CodingCookieRookie in [#18683](https://github.com/juju/juju/pull/18683)
* fix: panic in debug-log by @jack-w-shaw in [#18688](https://github.com/juju/juju/pull/18688)
* fix(jaasbakery): fix RefreshDischargeURL by @ale8k in [#18563](https://github.com/juju/juju/pull/18563)
* fix(ci): fix relation departing unit test on aws by @nvinuesa in [#18715](https://github.com/juju/juju/pull/18715)
* fix(tests): add workaround for checking output of discourse-k8s charm action by @anvial in [#18718](https://github.com/juju/juju/pull/18718)
* fix(simpleconnector): fix connect() method of simple connector to handle DialOptions by @ale8k in [#18358](https://github.com/juju/juju/pull/18358)
* fix: allow setting provisioning info for dying machine by @manadart in [#18500](https://github.com/juju/juju/pull/18500)
* fix: disambiguate k8s artefacts used for juju secrets by @wallyworld in [#18675](https://github.com/juju/juju/pull/18675)
* fix: backport azure image lookup fix by @anvial in [#18745](https://github.com/juju/juju/pull/18745)
* fix: cleanup k8s secret artefacts on model deletion by @wallyworld in [#18673](https://github.com/juju/juju/pull/18673)
* fix: find azure address prefix from new api result; by @ycliuhw in [#18776](https://github.com/juju/juju/pull/18776)
* fix: add recent introduced aws regions to update public clouds by @CodingCookieRookie in [#18774](https://github.com/juju/juju/pull/18774)
* fix: reflecting watcher in error handling by @hpidcock in [#18791](https://github.com/juju/juju/pull/18791)
* fix: upgrade go version to 1.23.6 to address GO-2025-3447 vuln by @nvinuesa in [#18832](https://github.com/juju/juju/pull/18832)
* fix: correctly handle path segments in controller URL by @kian99 in [#18703](https://github.com/juju/juju/pull/18703)
* fix: allow authorized external users to add clouds by @alesstimec in [#18858](https://github.com/juju/juju/pull/18858)
* fix: use after release by @SimonRichardson in [#18868](https://github.com/juju/juju/pull/18868)
* fix: parse corrected spelling of gratuitous-arp in Netplan by @manadart in [#18918](https://github.com/juju/juju/pull/18918)
* fix: correct case of JSON/YAML field name for FilesystemInfo.Attachments by @benhoyt in [#18931](https://github.com/juju/juju/pull/18931)
* fix(applicationoffers): handle permission validation correctly by @gfouillet in [#18928](https://github.com/juju/juju/pull/18928)
* fix: ensure 'app.kubernetes.io/name' label is set for user secrets by @wallyworld in [#18950](https://github.com/juju/juju/pull/18950)
* fix: GetModelInfo method by @alesstimec in [#18922](https://github.com/juju/juju/pull/18922)
* fix: copy mgo session when bulk deleting secrets by @wallyworld in [#18953](https://github.com/juju/juju/pull/18953)

🥳 New Contributors:
* @lengau made their first contribution in [#18670](https://github.com/juju/juju/pull/18670)
* @rthill91 made their first contribution in [#18656](https://github.com/juju/juju/pull/18656)
* @samuelallan72 made their first contribution in [#18365](https://github.com/juju/juju/pull/18365)
* @YanisaHS made their first contribution in [#18903](https://github.com/juju/juju/pull/18903)


### 🔸 **Juju 3.6.2**
🗓️ 21 Jan 2025
⚙️ Features:
* feat: add relation-model-get hook command by @wallyworld in [#18444](https://github.com/juju/juju/pull/18444)

🛠️ Fixes:
* fix: poor error message validating constraints by @CodingCookieRookie in [#18447](https://github.com/juju/juju/pull/18447)
* fix: do not set provider addresses for manually provisioned machines by @manadart in [#18535](https://github.com/juju/juju/pull/18535)
* fix: juju ssh enforcing port 22 by @CodingCookieRookie in [#18520](https://github.com/juju/juju/pull/18520)
* fix: improve error messages for register --replace by @wallyworld in [#18513](https://github.com/juju/juju/pull/18513)
* fix: cater for leadership change during secret drain by @wallyworld in [#18556](https://github.com/juju/juju/pull/18556)


### 🔸 **Juju 3.6.1**
🗓️ 11 Dec 2024
⚙️ Features:
* feat: bump pebble version to v1.17.0 by @benhoyt in [#18462](https://github.com/juju/juju/pull/18462)
* feat(cmd-register): prevent replacing existing controller if logged in by @ca-scribner in [#18079](https://github.com/juju/juju/pull/18079)
* feat: remove upgradesteps API client by @manadart in [#18374](https://github.com/juju/juju/pull/18374)
* feat: do not require upgradesteps API for migrations by @manadart in [#18387](https://github.com/juju/juju/pull/18387)

🛠️ Fixes:
* fix: do not fail probes during controller outage by @hpidcock in [#18468](https://github.com/juju/juju/pull/18468)
* fix: allow `refresh --base` to pivot a charm by @jameinel in [#18215](https://github.com/juju/juju/pull/18215)
* fix: fix bootstrap issue on k8s snap by @wallyworld in [#18366](https://github.com/juju/juju/pull/18366)
* fix: azure panic by @jack-w-shaw in [#18345](https://github.com/juju/juju/pull/18345) [#18346](https://github.com/juju/juju/pull/18346) [#18371](https://github.com/juju/juju/pull/18371)
* fix: qualify azure role definition with subscription by @wallyworld in [#18438](https://github.com/juju/juju/pull/18438)
* fix(ha): ignore virtual IP CIDR/32 by @gfouillet in [#18297](https://github.com/juju/juju/pull/18297)
* fix(logforwarder): add Close method to LogStream interface by @gfouillet in [#18278](https://github.com/juju/juju/pull/18278)
* fix(state): add assertion on the number of relations when adding relations by @alesstimec in [#18288](https://github.com/juju/juju/pull/18288)
* fix: fallback to env config when no base set by @SimonRichardson in [#18355](https://github.com/juju/juju/pull/18355)
* fix(login): use nil instead of empty user tag for NewLegacyLoginProvider by @gfouillet in [#18290](https://github.com/juju/juju/pull/18290)
* fix(ec2): remove auto assigned public IP when constraint is false by @nvinuesa in [#18432](https://github.com/juju/juju/pull/18432)


### 🔸 **Juju 3.6.0**
🗓️ 26 Nov 2024
⚙️ Features:
* Rootless charms on k8s
* Azure managed identities
* Idempotent Secrets
* The default base was bumped up to noble 24.04

🛠️ Fixes:
See the full list in these milestone pages:
* [RC2](https://launchpad.net/juju/3.6/3.6-rc2)
* [RC1](https://launchpad.net/juju/3.6/3.6-rc1)




## ⭐ **Juju 3.5**
> 30 Apr 2025: end of security fix support
>
> 28 Feb 2025: end of bug fix support

```{caution}

Juju 3.5 series is in security maintenance until 30 Apr 2025

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


### 🔸 **Juju 2.9.50**
🗓️ 25 Jul 2024

🛠️ Fixes:

- Fix [CVE-2024-6984](https://www.cve.org/CVERecord?id=CVE-2024-6984)

See the full list in the [milestone page](https://launchpad.net/juju/+milestone/2.9.50).


### 🔸 **Juju 2.9.49**
🗓️ 8 Apr 2024

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

### 🔸 **Juju 2.9.44**
🗓️ 20 Jul 2023

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

### 🔸 **Juju 2.9.42**
🗓️ 7 Mar 2023

Fixes several major bugs in 2.9.42.

🛠️ Fixes:

- Juju forces specifying series on metadata.yaml ([LP1992833](https://bugs.launchpad.net/juju/+bug/1992833))
- LXD unit binding to incorrect MAAS space with no subnets crashes with error ([LP1994124](https://bugs.launchpad.net/juju/+bug/1994124))
- panic when getting juju full status ([LP2002114](https://bugs.launchpad.net/juju/+bug/2002114))
- max-debug-log-duration: expected string or time.Duration ([LP2003149](https://bugs.launchpad.net/juju/+bug/2003149))
- juju using Openstack provider does not remove security groups ([LP1940637](https://bugs.launchpad.net/juju/+bug/1940637))

See the full list in the [milestone page](https://launchpad.net/juju/+milestone/2.9.42).

### 🔸 **Juju 2.9.38**
🗓️ 17 Jan 2023

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

### 🔸 **Juju 2.9.26**
🗓️ 12 Mar 2022

This release includes a fix for broken upgrades coming from a deployment with cross model relations to multiple offers hosted on an external controller ([LP1964130](https://bugs.launchpad.net/bugs/1964130)).

🛠️ Fixes:

- 2.9.25 Upgrade Fails for Cross-Controller CMRs([LP1964130](https://bugs.launchpad.net/bugs/1964130))
- Unauthorized for K8s API during charm removal([LP1941655](https://bugs.launchpad.net/bugs/1941655))
- CRD creation fails in pod spec charms on juju 2.9.25([LP1962187](https://bugs.launchpad.net/bugs/1962187))
- Juju prompted for a password in the middle of a bundle deploy([LP1960635](https://bugs.launchpad.net/bugs/1960635))
- Unable to set snap-store-assertions on model-config ([LP1961083](https://bugs.launchpad.net/bugs/1961083))
    - Note: This fix changes how to use log labels in model-config, extra single quotes are no longer required: `juju model-config -m controller "logging-config=#charmhub=TRACE"`



See the full list in the [milestone page](https://launchpad.net/juju/+milestone/2.9.26).


### 🔸 **Juju 2.9.25**
🗓️ 24 Feb 2022

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

### 🔸 **Juju 2.9.22**
🗓️ 13 Dec 2021

🛠️ Fixes:

- Juju 2.9.9 fails to bootstrap on AWS ([LP](https://bugs.launchpad.net/bugs/1938019))
- controller migration is very hard when dealing with large deployments ([LP](https://bugs.launchpad.net/bugs/1918680))
- models not logging ([LP](https://bugs.launchpad.net/bugs/1930899))
- ceph-osd is showing as fail ([LP](https://bugs.launchpad.net/bugs/1931567))
- Bootstrap with Juju 2.8.11 breaks on LXD 4.0.8 ([LP](https://bugs.launchpad.net/bugs/1949705))
- juju ssh --proxy not working on aws when targeting containers with FAN addresses ([LP](https://bugs.launchpad.net/bugs/1932547))

See the full list in the [milestone page](https://launchpad.net/juju/+milestone/2.9.22).

### 🔸 **Juju 2.9.21**
🗓️ 3 Dec 2021

🛠️ Fixes:

- juju enable-ha fails to cluster on 2.9.18 manual machines ([LP](https://bugs.launchpad.net/bugs/1951813))
- juju storage events are missing JUJU_STORAGE_ID ([LP](https://bugs.launchpad.net/bugs/1948228))
- Juju failing to remove unit due to attached storage stuck dying ([LP](https://bugs.launchpad.net/bugs/1950928))
- Juju creates two units for sidecar CAAS application ([LP](https://bugs.launchpad.net/bugs/1952014))

See the full list in the [milestone page](https://launchpad.net/juju/+milestone/2.9.21).

### 🔸 **Juju 2.9.19**
🗓️ 23 Nov 2021

🛠️ Fixes:

- controller models with valid credentials becoming suspended ([LP](https://bugs.launchpad.net/bugs/1841880))
- FIP created in incorrect AZ for instance when bootstrapped against OpenStack. ([LP](https://bugs.launchpad.net/bugs/1928979))
- [2.9.16 & 2.9.17] juju trust gets lost if juju config is run on application ([LP](https://bugs.launchpad.net/bugs/1948496))
- mongo 4.4 has a multiline --version ([LP](https://bugs.launchpad.net/bugs/1949582))

See the full list in the [milestone page](https://launchpad.net/juju/+milestone/2.9.19).

### 🔸 **Juju 2.8.13**
🗓️ 11 Nov 2021

This release fixes various issues with Juju **2.8**

🛠️ Fixes:

- Juju ~~2.9.9~~ fails to bootstrap on AWS ([LP](https://bugs.launchpad.net/bugs/1938019))
- controller migration is very hard when dealing with large deployments ([LP](https://bugs.launchpad.net/bugs/1918680))
- models not logging ([LP](https://bugs.launchpad.net/bugs/1930899))
- ceph-osd is showing as fail ([LP](https://bugs.launchpad.net/bugs/1931567))
- Bootstrap with Juju 2.8.11 breaks on LXD 4.0.8 ([LP](https://bugs.launchpad.net/bugs/1949705))
- juju ssh --proxy not working on aws when targeting containers with FAN addresses ([LP](https://bugs.launchpad.net/bugs/1932547))

See the full list in the [milestone page](https://launchpad.net/juju/+milestone/2.8.13).

### 🔸 **Juju 2.9.18**
🗓️ 8 Nov 2021

🛠️ Fixes:
- agent cannot be up on LXD/Fan network on OpenStack OVN/geneve mtu=1442 ([LP1936842](https://bugs.launchpad.net/bugs/1936842))
- no way to declare a k8s charm with metadata v2 that doesn't need a workload container ([LP1928991](https://bugs.launchpad.net/bugs/1928991))
- Method to run an action in a workload container in sidecar charms ([LP1923822](https://bugs.launchpad.net/bugs/1923822) )

See the full list in the [milestone page](https://launchpad.net/juju/+milestone/2.9.18).

### 🔸 **Juju 2.9.17**
🗓️ 27 Oct 2021

This release introduces {ref}`telemetry` as a configurable option per model.
It also supports [more OCI image registry providers](https://discourse.charmhub.io/t/initial-private-registry-support/5079) for pulling images used for CAAS models.

🛠️ Fixes:
- Leader role not transferred when the inital leader goes offline ([LP](https://bugs.launchpad.net/bugs/1947409))
- if the primary node of an HA config goes down, the controller stops responding ([LP](https://bugs.launchpad.net/bugs/1947179))
- Trust permissions not ready on install hook in sidecar charms ([LP](https://bugs.launchpad.net/bugs/1942792))
- deployed application loses trust after charm upgrade ([LP](https://bugs.launchpad.net/bugs/1940526))

See the full list in the [milestone page](https://launchpad.net/juju/+milestone/2.9.17).

### 🔸 **Juju 2.9.16**
🗓️ 11 Oct 2021

🛠️ Fixes:

- Unable to deploy workloads to lxd cloud added to k8s controller ([LP](https://bugs.launchpad.net/bugs/1943265))
- memory usage leading to OOMs on controllers
- LXD bootstrap fails with "Executable /snap/bin/juju-db.mongod not found" ([LP](https://bugs.launchpad.net/bugs/1945752))
- Requested image's type 'virtual-machine' doesn't match instance type 'container' ([LP](https://bugs.launchpad.net/bugs/1943088))

See the full list in the [milestone page](https://launchpad.net/juju/+milestone/2.9.16).

### 🔸 **Juju 2.9.15**
🗓️ 28 Sep 2021

This release improves the robustness of repeated cross model relation setup / teardown.
There's also some improvements to how raft is used internally to manage leases.

🛠️ Fixes:

- ceph mon does not render data to ceph-rados after redployment of ceph-radosgw only ([LP](https://bugs.launchpad.net/bugs/1940983))
- Unable to remove offers when 2 endpoints are offered with the same application ([LP](https://bugs.launchpad.net/bugs/1873472))
- upgrading 2.9.12 to 2.9.13 gets stuck in 'raftlease response timeout' ([LP](https://bugs.launchpad.net/bugs/1943075))
- pod-spec uniter exits on pending action op when remote caas container died ([LP](https://bugs.launchpad.net/bugs/1943776))

See the full list in the [milestone page](https://launchpad.net/juju/+milestone/2.9.15).

### 🔸 **Juju 2.9.14**
🗓️ 14 Sep 2021

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

### 🔸 **Juju 2.9.12**
🗓️ 30 Aug 2021

🛠️ Fixes:

- Cross-model relations broken for CAAS ([LP](https://bugs.launchpad.net/bugs/1940298))
- Boot failure when `model-config` sets `snap-proxy` ([LP](https://bugs.launchpad.net/bugs/1940445))
- The `juju export-bundle` command gives error after upgrade ([LP](https://bugs.launchpad.net/bugs/1939601))
- Several updates for the Raft engine that handles leases. These are steps to address ([LP](https://bugs.launchpad.net/juju/+bug/1934524)), though that issue is not completely resolved.

See the full list in the [milestone page](https://launchpad.net/juju/+milestone/2.9.12).

### 🔸 **Juju 2.9.11**
🗓️ 17 Aug 2021

🛠️ Fixes:

- Resource downloads are very slow in some cases ([LP](https://bugs.launchpad.net/juju/+bug/1905703))
- Upgrading the mongodb snap causes controller to hang without restarting mongod ([LP](https://bugs.launchpad.net/juju/+bug/1922789))
- OpenStack provider: retry-provisioning doesn't work for `Quota exceeded for ...` ([LP](https://bugs.launchpad.net/juju/+bug/1938736))

See the full list in the [milestone page](https://launchpad.net/juju/+milestone/2.9.11).

### 🔸 **Juju 2.9.10**
🗓️ 3 Aug 2021

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

### 🔸 **Juju 2.9.9**
🗓️ 19 Jul 2021

🛠️ Fixes:

- Juju 2.9.8 tries to use an empty UID when deleting Kubernetes objects, and cannot remove applications ([LP](https://bugs.launchpad.net/bugs/1936262))
- The `juju-log` output going to machine log file instead of unit log file in Juju 2.9.5 ([LP](https://bugs.launchpad.net/bugs/1933548))
- Deployment of private charms is broken in 2.9 (was working in 2.8) ([LP](https://bugs.launchpad.net/bugs/1932072))
- [Windows] Juju.exe and MicroK8s.exe bootstrap error ([LP](https://bugs.launchpad.net/bugs/1931590))

See the full list in the [milestone page](https://launchpad.net/juju/+milestone/2.9.9).

### 🔸 **Juju 2.9.8**
🗓️ 13 Jul 2021

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

### 🔸 **Juju 2.9.7**
🗓️ 6 Jul 2021

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

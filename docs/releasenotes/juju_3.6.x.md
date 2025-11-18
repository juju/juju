(juju36x)=
# Juju 3.6 (LTS)
> April 2036: expected end of security fix support
>
> 1 May 2026: expected end of bug fix support

```{note}
Juju 3.6 series is LTS
```

### 🔸 **Juju 3.6.11**
🗓️ 21 Oct 2025

This is a bug fix release primarily to address 2 regressions in the 3.6.10 release. As such, the 3.6.10 release is revoked.

The release notes below document the entire change set from 3.6.9.

As well as the regression fixes, the 3.6.11 release also contains updates to the secrets backend to improve the performance of dealing
with 1000s of secret revisions.

For a detailed list of every commit in this release, refer to the [Github 3.6.11 Release Notes](https://github.com/juju/juju/releases/tag/v3.6.11).

⚙️ Features:

#### New Google Cloud provider functionality
The Google Cloud provider gains support for various features already available on other clouds like AWS or Azure.
Specifically, the following features are now available:<br>

**VPC selection**<br>
Use the `vpc-id` model config value to select the name of the VPC to use for that model.
This is supplied when the model is added:<br>
`juju add-model mymodel --config vpc-id=myvpc`<br>
or at bootstrap for the controller model:<br>
`juju bootstrap google mycontroller --config vpc-id=myvpc`

For a VPC to be used when bootstrapping, there must be a firewall ruling allowing SSH traffic.

**Spaces support and subnet placement**<br>
Compute instances can now be provisioned such that space constraints and subnet placement can be used.
Subnet placement can use either the subnet name or subnet CIDR.<br>
eg<br>
`juju deploy mycharm --constraints="spaces=aspace"`
`juju deploy mycharm --constraints="subnet=asubnet"`
`juju deploy mycharm --constraints="subnet=10.142.0.0/16"`

**ssh-allow model config**<br>
The `ssh-allow` model config value is now supported. When specified, a firewall rule
is created to control ingress to the ssh port.<br>
eg<br>
`juju model-config ssh-allow="192.168.0.0/24"`

**Service account credentials**<br>
Similar to using instance roles on AWS or managed identities on Azure, it's now possible to use 
service accounts to confer permissions to Juju controllers such that a credential secret is not required.
The service account to be used must have the following scopes:
- `https://www.googleapis.com/auth/compute`
- `https://www.googleapis.com/auth/devstorage.full_control`

If you are not on a jump host, you must bootstrap using a standard credential and specify the service account like so:<br>
`juju bootstrap google mycontroller --bootstrap-constraints="instance-role=mydevname@2@developer.gserviceaccount.com"`

If you are on a jump host, you can set up a credential using `juju add-credential google`. Select credential type
`service-account` and enter the service account email. Then bootstrap as normal:<br>
`juju bootstrap google mycontroller`

**Constrains with `image-id`**<br>
The use of the `image-id` constraint value is now supported.<br>
eg<br>
`juju deploy mycharm --constraints="image-id=someimageid"`

* feat: add vpc support to the google provider by @wallyworld in https://github.com/juju/juju/pull/20518
* feat: add support for ssh-allow model config for gce by @adglkh in https://github.com/juju/juju/pull/20511
* feat: add support for gce service account credentials by @wallyworld in https://github.com/juju/juju/pull/20585
* feat: add spaces and subnet placement for gce by @wallyworld in https://github.com/juju/juju/pull/20568
* fix: support image-id constraint for charm deployment on gce by @adglkh in https://github.com/juju/juju/pull/20591

#### Preview: support for `--attach-storage` on Kubernetes models
For this release, a feature flag needs to be used as the feature is in preview.
On the client machine used to run the Juju CLI, simply:<br>
`export JUJU_DEV_FEATURE_FLAGS=k8s-attach-storage`

The primary use case this feature is designed to solve is to provide the ability to re-use volumes that have
been restored from a backup and attach them to units when deploying or scaling. The basic steps follow the 
usual import and attach workflow supported already on other clouds:
1. Import the PV into the Juju model to create a detached storage instance.
2. Use the imported storage with the `--attach-storage` option for `deploy` or `add-unit`.<br>
eg
```shell
juju import-filesystem kubernetes <mypvname>
# See the resulting detached storage instance.
juju status --storage
juju deploy postgresql-k8s --trust --attach-storage <storagename>
```

Note that if the PV to be imported still has a reference to a PVC, the `--force` option is needed when importing since the
existing claim reference will be removed.

* feat: import filesystem force option by @jneo8 in https://github.com/juju/juju/pull/20247
* feat: caas scale unit attach storage by @jneo8 in https://github.com/juju/juju/pull/20434

#### Removal of charm metrics
Charm metrics are no longer supported. This means that the `collect-metrics` and `meter-status-changed` hooks
will no longer fire and the `add-metrics` hook command becomes a no-op. In addition, the Juju CLI commands
`metrics`, `collect=metrics`, and `set-meter-status` are removed.

* feat: remove metrics and meterstatus functionality by @adisazhar123 in https://github.com/juju/juju/pull/20520

#### Documentation improvements
Many documentation improvements have been done for this release. Highlights include:
- better support for diagrams in dark mode
- visual enhancements to tips and notes
- removal of command aliases from CLI doc to reduce clutter
- hook and storage reference doc improvements
- add missing model configuration attributes

* fix: update secret-set hook tool help by @dimaqq in https://github.com/juju/juju/pull/20567
* docs: update starter pack by @tmihoc in https://github.com/juju/juju/pull/20419
* docs: break up manage your deployment by @tmihoc in https://github.com/juju/juju/pull/20403
* docs: update tutorial cf user testing by @tmihoc in https://github.com/juju/juju/pull/20384
* docs: adds to charms best practices concerning respecting model-config  settings by @addyess in https://github.com/juju/juju/pull/20268
* docs: fix cli docs -- links and formatting by @tmihoc in https://github.com/juju/juju/pull/20451
* docs: clean up storage ref doc by @tmihoc in https://github.com/juju/juju/pull/20501
* docs: 3.6+ update see note style by @tmihoc in https://github.com/juju/juju/pull/20523
* docs: 3.6+ fix relation images in dark mode by @tmihoc in https://github.com/juju/juju/pull/20539
* docs: 3.6+ improve structure and look of our hook doc by @tmihoc in https://github.com/juju/juju/pull/20553
* docs: 3.6+ revisit information foregrounding notes  by @tmihoc in https://github.com/juju/juju/pull/20551
* docs: add caution banner for 3.5 EOL by @nvinuesa in https://github.com/juju/juju/pull/20570
* doc: remove command aliases from cli reference doc by @wallyworld in https://github.com/juju/juju/pull/20584
* feat(docs): export dark-mode SVGs in Excalidraw converter by @anvial in https://github.com/juju/juju/pull/20420
* docs: add client ref, add changes from autogen, fix formatting in ref… by @tmihoc in https://github.com/juju/juju/pull/20572
* docs: add missing model configs by @tmihoc in https://github.com/juju/juju/pull/20678
* docs: add assets and data flows to security doc by @tmihoc in https://github.com/juju/juju/pull/20731
* docs: stress the importance of secret removal by @manadart in https://github.com/juju/juju/pull/20792

🛠️ Fixes:

#### Juju infrastructure
The commits below fix a regression in 3.6.10 which could in some circumstances cause a deadlock when closing
a web socket connection.
* fix: close response body when we get an error by @manadart in https://github.com/juju/juju/pull/20732
* fix: add close http body linter and address warnings by @kian99 in https://github.com/juju/juju/pull/20761
* fix: set IO deadlines without lock on websocket close by @manadart in https://github.com/juju/juju/pull/20744
* feat: tighten up API client closure by @manadart in https://github.com/juju/juju/pull/20778

#### Secrets
The commits below contain a fix to ensure obsolete secret revisions are purged from unit state, preventing unbounded
growth when individual revisions are purged. There's also a fix to secret deletion to prevent removal of secret
revisions partially matching the one asked for. Included as well are various performance improvements to better
handle 1000s of secret revisions.

* fix: add missing model-uuid index to secretRevisions by @manadart in https://github.com/juju/juju/pull/20688
* fix: secret collection indexes by @manadart in https://github.com/juju/juju/pull/20695
* fix: add a script for cleaning up unitstate by @jameinel in https://github.com/juju/juju/pull/20783
* fix: make secret-remove hook command idempotent by @wallyworld in https://github.com/juju/juju/pull/20796
* fix: purge obsolete revision from unit state when revision is removed by @wallyworld in https://github.com/juju/juju/pull/20862
* fix: allow secret-remove --revision to be called multiple times by @jameinel in https://github.com/juju/juju/pull/20806
* refactor: improve regex used to query secret artefacts in state by @wallyworld in https://github.com/juju/juju/pull/20829
* refactor: improve marking of orphaned revisions by @wallyworld in https://github.com/juju/juju/pull/20861
* refactor: optimise secret + revision metadata fetching for units by @hpidcock in https://github.com/juju/juju/pull/20878
* feat: add a script for cleaning up obsolete secrets by @jameinel in https://github.com/juju/juju/pull/20720

#### Juju refresh command
In some cases, the `juju refresh` command could panic.

* fix(refresh): fix panic in juju refresh when the channel is nil by @SimoneDutto in https://github.com/juju/juju/pull/20868

#### Openstack
The 3.6.9 release introduced a [regression](https://github.com/juju/juju/issues/20513) when running on Openstack clouds where security groups are disabled.
 
* fix: gracefully handle error when security group is disabled in openstack by @adisazhar123 in https://github.com/juju/juju/pull/20548

#### Google cloud
Specifying non-default disk storage using storage pools is fixed.
Using images configured for pro support is fixed. 

* fix: ensure gce images are correctly configured for pro support by @wallyworld in https://github.com/juju/juju/pull/20417
* fix: set maintenance policy upon instance creation on GCE by @adglkh in https://github.com/juju/juju/pull/20509
* fix: use `disk-type` instead of `type` when querying disk type on gce by @adglkh in https://github.com/juju/juju/pull/20557

#### Kubernetes
The mutating web hook created a misnamed label on pods which cause a regression when deploying certain charms. 

* fix: mutating web hook now attaches correct labels to k8s app resources by @wallyworld in https://github.com/juju/juju/pull/20774

Destroying a kubernetes controller could sometimes result in an error.

* fix: destroy/kill-controller for CAAS environs by @luci1900 in https://github.com/juju/juju/pull/20758

Deletion of applications deployed using sidecar charms now also deletes any Kubernetes resources created directly by the charm
and not managed by Juju. These include:
- custom resource definitions
- config maps
- deployments, daemonsets
- etc

Adding multiple secrets simultaneously could result in an error and this has been fixed.
Fixes for issues scaling applications:
- fix logic to only consider units >= target scale for removal, preventing inappropriate scaling during scale-up scenarios.
- only initiate scaling when all excess units (>= target) are dead.
The `credential-get` hook command now works on Kubernetes models for trusted applications the same way as for VM models.

* fix: crd resource cleanup on app removal by @CodingCookieRookie in https://github.com/juju/juju/pull/20385
* fix: caas reconcile scale up prevention by @jneo8 in https://github.com/juju/juju/pull/20479
* fix: adding multiple secrets simultaneously error by @CodingCookieRookie in https://github.com/juju/juju/pull/20401
* fix: crd cleanup on app removal for all other resouces by @CodingCookieRookie in https://github.com/juju/juju/pull/20489
* fix: allow "credential-get" to work on a K8s model by @benhoyt in https://github.com/juju/juju/pull/20428
* fix(k8s): call broker.Destroy when NewModel fails for CAAS models by @SimoneDutto in https://github.com/juju/juju/pull/20639

#### LXD
When a model is deleted, any LXD profiles created for the model and its applications are now removed.
The profile naming scheme has been updated to include a reference to the model UUID as well as name to ensure
profiles are fully disambiguated. Upon upgrade to this Juju version, existing profiles are renamed as needed. 

The new profile names are of the form:<br>
`juju-<model>-<shortid>` or `juju-<model>-<shortid>-<app>-<rev>`

* fix: cleanup LXD profile when a model is deleted by @adisazhar123 in https://github.com/juju/juju/pull/20357


🥳 New Contributors:
* @addyess made their first contribution in https://github.com/juju/juju/pull/20268
* @iasthc made their first contribution in https://github.com/juju/juju/pull/19765
* @claudiubelu made their first contribution in https://github.com/juju/juju/pull/19794
* @adglkh made their first contribution in https://github.com/juju/juju/pull/20509
* @dimaqq made their first contribution in https://github.com/juju/juju/pull/20567


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


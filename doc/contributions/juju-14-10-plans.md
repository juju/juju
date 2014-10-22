title: Juju 14.10 Plans

[TOC]

# Core

## Multi-environment State Server

Multi-environment, multi-customer.

### Use Cases

- Embedding in Azure - people can spin up an environment without paying for a state-server, quite a cost for people to start.
- Embedding in Horizon (OpenStack dashboard).

### How do we start?

- Create multiple client users (some sort of api) aka User Management.
- create-environment (need environments), list environments.
	- SelectEnvironment is called after Login, Login itself exposes the multi-environment API root. To avoid an extra round trip, Login can optionally pass in the EnvironmentUUID.
- Credentials need to move out of environment.
- Machine/user/etc (everything) documents gain an environment id (except users).
- API filters by environment id inherited from the SelectEnvironment.
- rsyslog needs to split based on tenant.
- provisioner/firewaller/other workers  gets environment id, one task per environment.
- Consider doing with the accounts/environment separation from `environments.yaml` (Juju 2.0 conf).
	- This is changing the DB representation so that we represent Environments referencing Accounts and pointing to Providers.
	- It may be possible the EnvironConfig still collapses this into one-big-bag of config, but it should be possible to easily change your Provider Credentials for a given Account have that cascade to all of your environments.

### Work Items

- State object gains an EnvironmentUUID attribute, all methods against that State object implicitly use that environment.
- Update state document objects (machine,unit,relation-scopes,etc) to include EnvironmentUUID.
- MultiState object
	- Includes the Users and Environment collection.
	- Used for initial Login to the API, and subsequent listing/selecting of environments.
	- SelectEnvironment returns an API root like we have today, backed by a State object (like today) that includes the environment UUID.
	- **Unclear**: how to preserve compatibility with clients that don’t pass the environment UUID.
	- Desirable: being able to avoid the extra round trip for Login+SelectEnvironment for most commands that know ahead of time (`status`, `add-unit`,etc).
- Admin on State server gives you the global rights across all Environments.
- Environments collection.
- MultiState APIs
	- `ListEnvironments`
		- Needs to filter based on the roles available to the user in various environments. Should not return environments that you don’t have access to.
	- `SelectEnvironment`
	- `CreateEnvironment`
	- `DestroyEnvironment`
- Logging
	- TBD, regardless the mechanism, we need environment UUID recorded per log message, so we can filter it out again.
	- In rsyslog it could be put into the prefix, or sharded into separate log files. 
- Include the GUI for the environment on (in) the state server per environment.

## HA

- Current Issues
	- `debug-log` retrieves the log from one API server; so in case of an HA environment not all logs are retrieved.
	- https://bugs.launchpad.net/juju-core/+bug/1310268
- What is missing?
	- HA on local.
- Next steps
	- Decrease count (3 -> 1, 5 -> 3).
	- Scaling API separately from mongo.

### Notes

Working on rsyslog.  Working logging to multiple rsyslogd.  This is ready to be reviewed.

Need to update the conf when machines added or removed.  This needs to be done.

Possible problem: logs being very out of order (hours off).

**Bug**: Peergrouper log spam on local.

HA on local can’t work 100% because VMs can’t start new VMs, so only machine 0 can be a useful master state server.  However, there are other tests that can be done with HA that would be useful on local HA. 

It would be useful to be able to have the master state server be beefy and higher priority for master, and the non-masters be non-beefy, because the master has a ton more load than the non-masters.  Right now, ensure availability is very broad and vague.  It’s not tweakable. However, you can do it by bootstrapping with a big machine, change the constraints to smaller machines, then ensure availability.  The only thing we would need to add is a way to give a state server a higher priority for becoming master.

Need better introspection to the status so that the GUI can better reflect what’s going on.  Need GUI to be able to call ensure availability.
GUI needs to show state servers.

Restore process for HA is just restore one machine then call ensure availability.

### GUI needs

- allwatcher needs to add fields for HA status changes.
- GUI needs to know what API Address to talk to, handle fallback when one goes away, and keep up to date to know who else to talk to.
- ensure-availability needs to return more status (actions triggered).
- How is HA enabled/displayed in GUI? what does machine view show?
- Can you deploy multiple Juju-GUI charms for HA of the GUI itself?

### CI

1. shutdown the master node or temporarily cripple the network to verify HA resolves the returned master.
2. Test on local because local will be used in demonstrations.
3. If backup-restore is also being done, then a restore of the master is a new master; ensure-availability must be rerun.

### Work Items

- **Bug**: Agent conf needs to store all addresses (hostports), not just private addresses.  Needed for manual provider.
- **Bug**:  Peergrouper log spam on local.
- Change mongo to write majority, this is a change per session.
- Change mongo to write WAL logs synchronously.
- Need docs about how to use ensure availability on how to remove a machine that died. (try to improve the actual user story for how this works).
- `juju bootstrap` && `juju ensure-availability`  (should not try to create a replacement for machine-0).
- Set up all status on bootstrap machine during bootstrap so it is created in a known good state and doesn’t start up looking like it’s down.
- Machine that was down, ensure-availability was run to replace it, when the machine comes back it should not have a vote and should not try to be another API server.
- “juju upgrade-juju” should coordinate between the API servers to enable DB schema updates (before rewriting the schema make sure all API servers are upgraded and then only the master API server performs the schema change).
- APIWorker on nodes with JujuManageEnvironment should only connect to the API Server on localhost.
- Determine how Backup works when in HA.
- Changes for GUI to expose HA status.
- Changes for GUI to monitor what the current API servers are (need the Watcher that other Agents use exposed on the Client Facade).
- `ensure-availability` needs to return more status (actions triggered) (EnsureAvailability API call should return the actions).
- Change mongo to write majority, this is a change per session.
- Change mongo to write WAL logs synchronously.
- Need docs about how to use ensure availability on how to remove a machine that died.
- `juju bootstrap && juju ensure-availability`
- Set up all status on bootstrap machine during bootstrap so it is created in a known good state and doesn’t start up looking like it’s down.

### Work items (stretch goals)

- Ability to reduce number of state servers.
- Handle problem with ensure availability getting called twice in a row (since new servers aren’t up yet, we start more new state servers).
- Ability to set priority on a state server.
- Ability to reduce number of state servers.
- Autorecovery - bringing back machines that die (or just calling  ensure availability again).
- Handle problem with ensure availability getting called twice in a row (since new servers aren’t up yet, we start more new state servers).

## State, status, charm reporting

Statuses like ‘started’ don’t have enough detail. We don’t know the true state of the system or a charm from status like started.

- s/ready/healthy   and s/unready/unhealthy
- Add jujuc tools ready and unready (healthy, unhealthy).
	- Ready takes no positional arguments.
	- Unready takes a single positional argument that is a message that explains why.
	- Charm authors choose the message they want to use.
	- Both ready/unready, when called without other flags, apply to the unit that is running.
	- The above also accept a relation flag, `-r <relation id>`, which applies the status to the specified relation.
	- The status data for a unit keeps track of the ready status, expose in status.
	- Implementation needs to be shared with allwatcher so gui gets to see the info.
- Implement a ready-check hook that will be called periodically if exists; units expected to update ready status to be reported when hook is called.
- The details states are sub-statuses of ‘started’.
- Possible granular statuses for units.
	- provisioned
	- installing (sub or pending)
- Juju will poll the ready-check hook for current state. Charms need to respond ready or unready.
- We might want a concise and summary of the status. GUI might want to show the concise and later show the summary.
- Status is already bloated.
	- Can status be intelligent enough to only include the data needed?
	- Can you subscribe to get updates for just the information you think is changing...subscribe to the allwatcher?
- `juju status --all` would be the current behavior.
	- We would `start --all` being implicit, but depreciated.
	- We will switch to a more terse format.
- The status “started” is not really ready.
	- There may be other hooks that still need to run.
	- Only the charm knows when the service is ready.
- When install completes, the status is implicitly “started”.
	- The charm author can set install to return a message to mean it is unready.
- Authors want to know when a charm is blocked because it is waiting on a hook.
	- We can solve 80% of the problem with some effort but a proper solution is a lot of work.
	- It isn’t clear when one unit is still being debugged.

### Work Items

1. Introduce granular statuses.
1. Implement filters/subscribers to retrieve granular status.
1. Unify status and all-watcher.
1. Switch status from --all to the concise form
	- (?) know when the charm is stable, when there are no hooks queued 
 	- (?) know when all services are stable
1. When deploying then adding debug-hooks, the later could set up a pinger for the service being deployed, which puts the service into debug as it comes up.
1. `juju retry` to restart the hooks because resolved is abused.

## Error Handling

- JDFI. We have a package. Use it.
- We need to annotate with a line number and a stacktrace.
	- We have type preservation.
- There are some agreement to change the names of some of the API.
- Add this as we needed. Switching all code to use it is stalling the production line.
- Reviewer will push back to use the new error handling.

### Work Items

1. Extend juju errors package to annotate with file and line number.
1. Log the annotated stack trace.
1. Change the backend to use `errgo`.
1. We need a template (Dimiter’s example) of how to use error logging.

## Image Based Workflows

Charms able to specify an image (maybe docker) with the addition of storage, storage dirs are passed into docker as it is launched.

Unit agent may run either inside or outside the docker container (not yet determined).

Machine agent would mount the storage, and the charm directory into the docker container when it starts. The hooks are executed the docker container.

Looking to make the docker support a first class citizen in Juju.

*“Juju incorporates docker for image based workflows”*

Maybe limited to ones based on ubuntu-cloud image (full OS container).

May well have a registry per CPC to make downloading images faster on that cloud.

Perhaps have docker file (instructions to build the image) into the charm.  The registry that we look up needs to be configurable.

Offline install will require pulling images into a local registry.

### Work Items

1. Unit agent inside container.
1. Image registry.
1. Charm metadata to describe the image and registry.
1. Deployer to understand docker, deployer inspects charm metadata to determine deployment method, traditional vs. docker.
1. A docker deployer needs to be written that can download the image from a registry, and start the container mounting the agent config, storage, charm dir, upstart script for unit agent (if unit agent inside).
1. Need docker work to execute hooks inside the container from the outside.

**Depends on storage 0.1 done first.**

## Scalability

### Items that need discussion in Vegas

- How do we scale to an environment with 15k active units? 
	- How do admin operations scale? 
	- How do we handle failing units? 
		- dump and re-create
		- Interaction with storage definition.
	- How do we make a “juju status” that can provide a summary without getting bogged down in repeated information
	- How does relation/get/set change propagation scale? 
	- Where are the current bottlenecks when deploying say hadoop? 
	- Where are the current bottlenecks when deploying OpenStack? 
- Pub/Sub
	- What do we need to do here? 
	- Notes:
		- We need a pub/sub for the watchers to help scale.
		- Each watcher pub/subs on its own, move up one level?
		- Need for respond to events that occur, in a non-coupled way (indirect sub to goroutine).
		- Logging particular events? 
		- Only one thing looking at the transaction log, whoops, not as bad as we thought.
		- 100k units, leads to millions of go-routines, blocking is an issue.
		- If we do a pub/sub system, let’s use it everywhere? Replace watchers?
		- Related to idea of pub/sub on output variables and the like it sounds like.
		- Watching subfield granularity of a document perhaps?
		- 0mq has this, should reuse that and not invent our own pub/sub.
		- 0mq has Go bindings, wonder if it works in gccgo.
		- Does this replace the api? No, can’t Javascript to 0mq directly so need some api-ness for clients.
		- Are there alternatives to the watcher design?
		- Really good for testing. Can decouple parts and make it easy/fast to test if the event is fired.
		- Shared watcher for all things (On the service object?)
		- Have a big copy of the world in memory, helps with a lot of this.
		- Charm output variables watching, charm outputs, hits state, megawatcher catches and updates and tells everyone it’s changed.
		- Helps with ABA problem using in memory model.
		- Use 3rd party pub-sub rather than writing our own.

### Work Items

1. Boot generic machines which then ask juju for identity info.
1. Bulk machine provisioning.
1. Fix uniter event storm due to “number units” changed events.
1. Implement proper pub sub system to replace watchers.
1. State server machine agent (APIServer) should not listen to outside connection requests until it itself (APIWorker) has started.

## Determinism

### First Issue: Install repeatability

There are two approaches to giving us better isolation from network and other externalities at deploy time.

1. Fix charms so they don’t have to rely on external resources.
	- Perhaps by improvements around fat.
		- REQUIRED: Remove internal Git usage (DONE).
	- Perhaps by making it easy to manage those resources in Juju itself? 
		- Either create a TOSCA like “resources” catalog per environment: upload or fetch resources to the environment at deploy time (or as a pre-deploy step)
		- or create a single central resource catalog with forwarding aka “gem store for the world”.
1. Snapshot based workflows for scale/up/down so external resources aren't hit on every new deploy.
	- We could add the hooks necessary to core, but the actual orchestration of images, seems a bit more tricky and could depend on a better storage story.
    
### Second Issue

From Kapil: “Runtime modification of any change results in non deterministic propagation across the topology that can lead to service interruption. Needs change barriers around many things but thats not implemented or available. e.g. config changed and upgrade for example executed at the same time by all units.”.
    
### Upgrade Juju

`juju upgrade-juju` -> goes to magic revision (simple bug fix) that an operator can’t determine.

Juju internally lacks composable transactions, many actions violate semantic transaction boundaries and thus partial failure states leave inconsistencies.

Kapil notes: 

> One of the issues with complex application topologies is how runtime changes ripple through the system. e.g. a config change on service propagates via relations to service b and then service c. It's eventually consistent and convergent, but during the convergence what's the status of the services within the topology. Is it operational? Is temporarily broken?

> **This is a hard problem** to solve and its one I've encountered in both our OpenStack and Cloud Foundry charms. 

> In discussions with Ben during the Cloud Foundry sprint, the only mitigation we could think of on Juju's part was some form of barrier coordination around changes. e.g. that the ripple proceeds evenly through the system. It's not a panacea but it can help. Especially so looking at simpler cases of just doing barriers around `config-change` and `charm-upgrade`. What makes this a bit worse for Juju then other systems, is that we're purposefully encapsulating behavior in arbitrary languages and promoting blind/trust based reuse, so a charm user doesn't really know what effect setting any config value will have. e.g. the cases I encountered before were setting a 'shared-secret' value and and an 'ssl' enumeration value on respective service config... for the ssl i was able to audit that it was okay at runtime.. but thats a really subtle thing to test or detect or maintain.

> Any change can ripple through the topology.  We have an eventually-consistent system, but while it is rippling, we have no idea.  Lack of determinism means someone who uses Juju cannot make uptime guarantees

**Bug**: downgrading charms is not well supported.

### Questions

- Do we need barriers?  e.g. config-changed affects all units of a service simultaneously.
- Do we need pools of units within a service?

### Work Items

- Unit-Ids must be unique (even after you've destroyed and re-created a service).
- Address changes must propagate to relations.
- `--dry-run` for `juju upgrade-juju`.
- `--dry-run` for deploy (what charm version and series am I going to get?).

## Health Checks

Juju “status” reporting in charm needs to be clearly defined and expressive enough to cover a few critical use cases.   It is important to note that BOSH has such a system. 

- Canaries and rolling unit upgrades (health check as pre-requisite).
- Is a service actually running?
- Coordination of database schema upgrades with webserver unit upgrades (as an example of the general problem of coordinated upgrades).
- Determining when HA Quorum has been reached or a server has been degraded. 

### Questions

- We discussed Error, and Ready as states, but do we need a third? Pending, Error, and Ready? 
- Do we need any more than three states? 
- Suggestion:  Three states, plus an error description JSON map. 

## Storage management

### Allow charms to declare storage needs (block and mount). 

- [Discussion from Capetown](https://docs.google.com/a/canonical.com/document/d/1akh53dDTROnd0wTjGjOrsEp-7CGorxVp2ErzMC_G-zg/edit)
- [Proposal post Capetown (MS) (lacks storage-sets)](https://docs.google.com/a/canonical.com/document/d/1OhaLiHMoGNFEmDJTiNGMluIlkFtzcYjezC8Yq4nAX3Q/edit#heading=h.wjxtdqqbl1fg)

Entity to be managed:

- CRUD, snapshot

Charms declare it:

- Path, type (ephemeral/persistent) block.

Storage 0.1:

- Storage set in state - track information in some way.
- Disks ( placement, storage).
- Provider APIs (to create, delete, attach storage, … expand for later).
- Provider to be able to attach storage to a machine.
- Charms need to be able to say in metadata.
- `jujud` commands to have charms be able to resolve where the storage is on the machine.
- Degradation, manual provider or other provider that doesn’t provide storage (DO), do not fail to deploy, but we need to communication warning of some form, CLI should fail? API will not?

Storage set, need to talk services, needs to be exposed as management processes.

Multitenant storage? Probably not for initial implementation, but ***do not design it out***.

Need to consider being able to map our existing storage policy onto the new design (e.g. AWS EBS volume for how Juju works with Amazon)

NOTE: Storage is tied to a zone, ops can take a long time to run.

Consider upgrades of charms, and how we can move from the existing state where a charm may have their own storage that they have handled, to the new world where we model the storage in state.

- (2) Add state storage document to charm document.
	- Upgrading juju should detect services that have charms with storage requirements and fulfill them for new units.
- (6) Add state for storage entities attached to units.
	- Lifecycle management for storage entities.
- (6) When deploying units, need to find out storage is needed.
	- Make provisioner aware of workloads and include storage details when needed.
	- Change unit assignment to machine based on storage restrictions.
- (4) Define provider apis for handling storage.
	- Create new volume.
	- Delete volume.
	- Attach volume to instance.
- (12) Implement provider APIs for storage on different providers.
	- OpenStack
	- EC2
	- MaaS
	- Azure?
- (0) Consider storage provider APIs for compute providers that have storage as a service.
- (2) Define new `metadata.yaml` fields for dealing with storage.
- (0) Consider mapping between charm requirements and service-level restrictions on what storage should actually be provided.
- (4) Add storage to status.
	- Top level storage entity keys.
	- Units showing storage entities associated.
	- Services show storage details.
- (4) CLI/API operations on storage entities.
	- Add storage.
	- Remove storage.
	- Further operations? Resize? Not now.

## Juju as a good auto-scaling toolkit

*Not a goal:  Doing autoscaling in core.*

Goal: providing the API’s and features needed to easily write auto-scaling systems for specific workloads. 

Outside stakeholders: Cloud Installer team. 

We need to be able to clean up after ourselves automatically. 
Where “clean up” actions are required, they need to take bulk operation commands. 

- Destroy-service/destroy-unit should cascade to destroy dirty machines

## IAAS scalability

- Security Group re-think: 
	- The security group approach needs to switch to per-service groups
	- We need to support individual on-machine/container firewall rules
- Support for instance type and AZ locality

## Idempotency

Is this a juju issue, or a charm issue?  Config management tools always promise this, but rarely deliver -- though many deliver **more** than juju.  What are the specific issues in question with Cloud Foundry?

## Charm "resources" (fat bundles, local storage, external resource caching)

### Problem Statements

- Writing and maintaining “fat” charms is difficult
	- Forking charm executables to support multiple upstream release artifacts is sub-optimal
	- Fat charms are problematic
- Non-fat charms are dependent on quite a few external resources for deployment
- Non-fat charms are not *necessarily* deterministic as to which version of the software will be installed (even to the point of sometimes deploying different versions in the same service)

### Proposed Agenda

- Discuss making “fat charms better” 
	- Switch to a “resources” model, where a charm can declare the ‘external’ content that it depends on, and the store manages caching and replication of it
- Consider building on the work IS has done
- Choose a path, and enumerate all the work that needs to be done to fully solve this problem

### Proposal

- ~`resource-get NAME` within a charm to pull down a published blob~
- Instead using a model where charms request names, the charm overall declares the resources it uses, and the Uniter ensures that the data is available before firing the upgrade/install hooks.
- `resources.yaml` declares a list of streams that contain resources

```
default-stream: stable
	streams:
		 stable:
		devel:
			common:
				common.zip
			amd64:
				foobar.zip
```
					
- Rresources directory structure for charms should match those of the charm author so bind-mounting the directory for development still works in the deployed version of the directory structure, you will only have common and arch specific files. Should there be a symlink to specific arch? Either:
	- publish charm errors if there are name collisions across common and arch specific directories. This way all the files are in a resources directory for the hook execution. This does mean that the charm developer needs a way to create symlinks in the top directory to the current set of resources they want to use (charm-tool resources-link amd64) - Windows? (they have symlinks right?)
	- charm has resources/common, and resources/arch. “arch” is still a link, but just one.
	- charm has resources/common, and resources/amd64 
		- this requires the hook knowing the arch
- charm identifiers become qualified with the stream name and resource version (precise/mysql-25.devel.325)
- juju status will show new version available if the entire version string (including resources) changes.
	- if mysql-25.devel.325 is installed, and a different version of resources becomes current, this will be shown in “juju status”
	- currently ask for mysql-latest, should perhaps be changed to mysql-current as we don’t necessarily want the latest version
- each named stream has an independent version, which is independent of both other streams and of the explicit charm version.
- upgrade-charm upgrades to the latest full version of the charm, including its resources
- upgrade-charm reports to the user what old version it was at and what the new version it upgraded to
- blobs are stored in the charm store, your environment always has a charm store, which can be synced for offline deployments
- today deploy ensures that the charm is available, copies it to environment storage, this will now need to do the same for the resources for the charm
- deploy should also confirm that the charm version and resource version are compatible
	- `juju deploy mysql-25.dev.326` may fail because resources version 326 has a different manifest than declared in charm 25’s `resources.yaml`
	- `juju deploy mysql`
		- finds the current version of the charm and resources in the default stream
		- the charm store has already validated that they match
	- `juju deploy mysql-25`
		- uses the default stream
		- how do we determine the resources for this version
			- Does current match? If yes, use it.
			- If 25 < current, then look back from current resources and grab the first that has a matching manifest.
			- Could just fail.
			- Charm store could track the best current resources for any given charm version, as identified by moving the current resources pointer while keeping the charm pointer the same. For charm versions that are current, remember the current resources version. 
			- If we take this approach, there will be charm versions that have never been “current”, so deploying this without explicitly specifying the resources version will fail.
	- `juju deploy mysql.nightly` (syntax TBD)
	- `juju deploy mysql --option=stream=nightly` (hand wave - we don’t like this one as getting full version partly from config feels weird)
		- Find the current version of mysql and the current version of the paid stream resources.
		- So, the charm store needs to remember the current resources for each stream for each charm version for the current values.
- charm store has pointers for “current” version of charms and “current” version of resources
- charm store requires that the resources defined in the current pointers have the same shape (same list of files)
- `charm-publish` requires a local copy of all resources (for all architectures), and validates that `resources.yaml` matches the resources tree.
- `charm-publish` computes the local Hash of resources, and the Manifest for what is currently in the charm store to publish both the charm metadata and all resources in a single request
	- publishing does not immediately move the ‘current’ pointer. This allows someone to explicitly deploy the version and test that the charm works with that version of resources
- supported architectures is an emergent property tracked by the charm store (known bad/unknown/known good) based on testing - hand wave
- charm store will be expected to de-dupe based on content hash (possibly a pair of different long hashes just to be sure)
	- don’t let the manifest just be the SHA hash without a challenge
		- either random set of bytes from the content
		- salted hash - charm store gives the salt, publish charm computes the salted hash to confirm that they actually have the content

### Spec

- be clear about what is in the charm store, what is defined by the charm in `resources.yaml`, and what deployed on disk
- use cases should show both the charm developer workflow, and user upgrade flow (which files get uploaded/downloaded etc)
- developing a new charm with resources
	- with common resources
	- with different resources for different architectures
	- with some architectures needing specific files
- upgrade a charm by just modifying a few files
- upgrade a charm by only modifying charm hook
- upgrade both hook and resources
- adding new files
- docker workflow with base image and one overlay
- updating the overlay of a docker charm
- adding a new docker overlay will cause a rev bump on the charm as well as the resources because the resources.yaml file has to change to include the new layer
	- illustrate explicitly the workflow if they forget to add the new layer to the resources.yaml file - publish fails because resources.yaml doesn’t match disk resources directory tree

### Discussion

- Canarying will have to be across charm revision, blob set, and charm config.
	- The charm version now includes charm revision and resources revision.
- Further discussion needed around health status for canaries later.
- Access control needs to be on top of the content addressing, just knowing the hash does not imply permission to fetch.
- Saving network transfer by doing binary diff on blobs of the same name with different hashes/versions would be nice for upgrades.
	*sabdfl* says we have this behaviour already with the phone images, and we should break this out into some common library somewhere somehow.

### Charms define their resources

- `resources.yaml` (next to `metadata.yaml`)
- Stream
	- Has a description (free vs paid, beta etc/stable vs proposed)
	- If you want to change the logic of a charm based on the blob stream, that is actually a different charm (or an if statement in your hooks)
	- Streams will gain ACLs later (can you use the paid stream)
	- Charms must declare a default stream
- Filenames
	- name of the blob
- Version
	- just a number (monotonically increasing number), version is stream dependent
	- store has a pointer to the “current” version (which may not be the latest)
- Architecture
- Charm declares the shape of the resources it consumes (what files must be available). The store maintains the invariant that when the resource is updated, it contains the shape that the charm declared.
- `charm-publish` uploads both the version of the charm and the version of the resources
- we add a “current” pointer to charms like the resources, so that you have an opportunity to upload the charm and its resources and test it before it becomes the default charm that people get (instead of getting the ‘latest’ charm, you always get the ‘current’ unless explicitly specified)
- mysql-13.paid.62

### Notes

We need to cache fat charms on the bootstrap node. We need to "auto Kapil" fat charms. Sometimes we don't even have access to the outside network. We need one hop from the unit to the bootstrap

However the important thing is that customers will probably fat charm everything, aka. huge IBM Websphere Java payload. 

- Can Juju handle gigs of payload? Nate: Yes, moving away from the git storage. 
- Is there anything core can do to make charms smaller?
	- Marco: Yes
	- Ben: We need a mechanism to specific common deps so that we can share them instead of having a copy in every charm. A bundle could have deps included, or maybe a common blob store?
- juju-deployer is moving to core.

If we move to image based workloads we can have a set image that included all the deps.

Nate: We could do it so if we’re on a certain cloud we can install the deps as part of the cloud: aka. if I am on softlayer make sure IBM java is installed via cloud-init. So we can do things like an optimized image for big data. 

### Work Items

1. Add optional format version to charm metadata (default 0) - 2
	- Get juju to reject charms with formats it doesn’t know about ASAP
1. Charm store needs to grow blob storage, with multiple streams, current resource pointers and links to the charm itself for the resources - 4
1. Charm store needs to gain current charm revision pointers to charm - 2
	- Juju should ask for current not latest
1. The charm store needs to know which revisions of each resource stream each charm revision works with - 2
1. Charm gains optional `resources.yaml` - 2
	- Bump format version for those using `resources.yaml`
1. Need to write a proper charm publish - 12
	- Resource manifest match
	- Salted hashes
	- Partial diff up/down not in rev 1
1. State server needs an HA charm/resources store - 8
	- Should use same code as the actual charm store (shared lib/package)
	- Replaces current charm storage in provider storage
1. Charm does not exist in state until we have copied all authorized resources into the local charm store. - 2
1. Uniter/charm.deployer needs to know about the resources file, parse content, know which stream, request resources from the local charm store, probably authenticated - 4
	- Puts the resources into the resources directory as part of Deploy
1. Bind mounting ensuring the links for the files flatten in the resources dir - 2

## Make writing providers easier

### Problems

- Writing providers is hard.
- Writing providers takes a long time .
- Writing providers requires knowledge of internals of juju-core.
- Providers suffer bitrot quite quickly.

### Agenda

- Can we externalize from core? (plugins/other languages?)
- Pre-made stub project with pluggable functions?
- How to keep in sync with core changes and avoid bitrot?
- How to insulate providers from changes to core?
- Can we simplify the interface?
- Complicating factor is config - can some be shared?
- Need to design for reuse - factor out common logic.

### Notes

- Keep `EnvironProvider`
- Split `Environ` interface into smaller chunks 
- E.g. `InstanceManagement`, `Firewall`
- Smaller structs with common logic, e.g. port management what use provider specific call outs
- Extract out Juju specific logic which is “duplicated” across providers and refactor into shared struct 
- Above will allow necessary provider specific call outs to be identified

### Work Items

1. Methods on provider operate on instance id
1. Introduce bulk API calls
1. Move instance addresses into environs/network
1. Split `Environ` interface into smaller chunks; introduce `InstanceManager`, `Firewaller`
1. Smaller structs with common logic, e.g. port management what use provider specific call outs
1. Extract out Juju specific logic which is “duplicated” across providers and refactor into shared struct 
1. Stop using many security groups - use default group with iptables
1. Use `LoadBalancer`? interface (needed by Azure); will provide open/close ports; most providers will not need this and/or return no-ops
1. `Firewaller` worker be made the sole process responsible for opening/closing ports on individual nodes
1. Refactor provider’s use of `MachineConfig` as means to pass in params for cloud init; consider ssh’ing into pristine image to do work as per manual provider?????

## Availability Zones

- Users want to be able to place units in availability zones explicitly (provider-specific placement directives). The core framework is nearing completion; providers need to implement provider-specific placement directives on top.
- Users want highly-available services (Juju infrastructure and charms). On some clouds (Azure), spreading across zones is critical; on others it is just highly desirable.
- Optional: one nice feature of the Azure Availability Set implementation is automatic IP load balancing (no need for HA Proxy which itself becomes a SPoF). Should we support this in other providers (AWS ELB, OpenStack LBaaS, ...)?

### Agenda

- Prioritise implementation across providers (e.g. OpenStack > MaaS > EC2?).
- Discuss overall HA story, IP load balancing.

Azure supports implicit load balancing but don’t care about other clouds for now.

### Work Items

1. Determine which providers support zones; EC2, OpenStack, Azure?
1. Implement distribution group in all providers; either they do it or return an error.
1. New policy in state which handles units on existing machines.
1. New method on state which accepts distribution groups and list of candidate instance ids and returns a list of equal best candidates.
1. Add API call to AMZ to find availability zones.

## Networks

- Juju needs to be aware of existing cloud-specific networks, so it can make them available to the user (e.g. to specify placement and connectivity  requirements for services and machines, provide network capabilities for charms/relations, fine-tuning relations connectivity, etc.).
- Juju needs to treat containers and machines in an uniform way with regards to networks and connectivity (e.g. providing and updating addresses for machines and containers, including when nesting).
- Knowing the network topology and infrastructure in the cloud, juju can have a better model how services/machines interact and can provide user-facing tools to manage that model (CLI/API, constraints/placement directives, charm metadata) in on a high level, so that the user doesn’t need to know or care how lower level networking is configured.

### Agenda

- Discuss and outline the high-level architecture integrating existing MaaS VLAN MVP work and instances addresses, so that we have a unified networking/addressability model.
- Prioritize implementation across providers.
- Discuss and define required features and deadlines?

### Meeting Notes

- We need networks per service -> then configure them on machines.
- Default networks get created (public/private)?
- Networks per relation -> routing between netdb (mysql) /netapp (wp) e.g.
- network relations to define routing ? add-net-relation netdb netapp; then when add-relation mysql wordpress [--using=netrel1] (if more than one)
- Container addressability

## Networking - Connections vs Relations

Discussion of specifics of networking routing.

- Relations do not always imply connections (although usually they do, except when they don’t like with proxy charms).
- Juju wants to model the physical connections to open ports/iptables/securitygroups/firewalls appropriately to allow the relation’s actual traffic.
- We need to be able to specify the endpoints for communication within charm hooks if it’s not the default model. Possible hook commands for that:
	- `enable-traffic endpoint_ip_address port_range`
	- For example: `enable-traffic 194.123.45.6 1770-2000`
	- `disable-traffic ep port_range`
- Also talk to OpenStack charmers about non relation TCP traffic.
- Should Juju model routing rules and tables for networks? (Directly via API/CLI or implicitly as part of other commands, like add-relation between services on different networks).

## Deployer into Juju Core

- To embed the GUI we need a solid path for making bundles work.
- You can’t juju deploy a bundle.
- Moving towards stacks Core should support bundles like charms, provide apis to the files inside, etc. 
- Can GUI use the ECS to replace the functionality of the deployer for GUI needs?

The goal if the meeting is to verify that this is a logical path forward and create a plan to migrate to this. Stakeholders should agree on the needs in Core and make sure that it works with vs against future plans to expand on the idea of bundles into fat bundles and stacks. 

## Bundles to Stacks

What’s needed to turn bundles into stacks?

Bundles have no identity at run time, we want this for stacks. A namespace to identify the group of services that are under a bundle.

Drag a bundle to the GUI, you get a bunch of services, with stacks, drag and drop a stack and you get one identifiable stack icon that itself is a composable entity and logical unit.

- Namespaces
	- The collection of deployed entities belong to a stack
	- Bundles today ‘disappear’ once deployed (the services are available, but there is no visible difference from just doing the steps manually)
- Exposed endpoints
	- Interface “http” on the stack is actually “http” on internal Wordpress
- Hierarchy (nesting)
- Default “status” output shows the collapsed stack, explicitly describing the stack shows the internal details

### GUI concerns/thoughts

- Expanded stack takes over the canvas, other items not shown
- Drag on an “empty stack” which you can explode to edit, adding new services inside

### Notes

- GUI can’t support bundles with local charms
- Bundles should become core entity supported by juju-core
- Deployer into juju-core should come after work for supporting uncommitted changes
- (dry run option?)

### Stacks 2.0

Further items about what a stack becomes

- Incorporating Actions
- Describing behavior for Add-Unit of a stack

### Work Items

Spend time to make a concrete Spec for next steps
for “namespacing” an initial implementation could just tag each item that is deployed with a name/UUID

## Charm Store 2.0

- Access Control
- Replacing Charm World
- Ingesting Charms (for example w/ GitHub)
- Ingesting Bundles
- Search

Kapil’s aim: simplify current model of charm handling. Break three way link between launchpad, charmworld (deals with bundles, used via api by the gui), and the charmstore (deals in charms, used by juju-core state server). Question: is breaking the link between launchpad and charmworld the first step?

Lots of discussion over first steps, migrate charmworld api into store? Does the state server also need to implement it? Currently api is small but specific, search, pull specific file (maybe with some magic for icons) out of charms, some other things.

**First step**: Add feed from store that advertises charms. Change charmworld ingest to read from the store feed rather than launchpad directly.

**Second step**: Bundles are only in charmworld currently. Pulled from launchpad, are a branch with bundles.yaml, a readme, similar to a charm. Store needs to ingest bundles as well and also publish as a separate bundle feed. Change charmworld ingest to read store bundle feed.

**Third step**: Add v4 api that supercedes current charmworld v3 api, implemented in store. Cleaning up direct file access and other odd things at the same time.  Remember that charm-tools are currently a consumer of v3 api.

We may want to split charm store out of juju-core codebase, along with packages such as charm in core to separate libraries.

After charmworld no longer talks to Launchpad it will be easier to provide ingestion from other sources, e.g. GitHub.  Publishing directly to the store will be possible also.

Work item - bac - document existing charmworld API 3 (see [Charmworld API 3 Docs](http://charmworld.readthedocs.org/en/latest/api.html))

We’ll need to be able to serve individual files out of charms:

- `metadata.yaml`
- `icon.svg`
- `README`

Search capability could be provided by Mongo 2.6 fulltext search?

### Questions

- How does ingestion of charm store charms for personal names space?
	- `juju deploy cs:gh`
- Charm store 2.0 should be able to ingest not only from GitHub but from a specific branch in a GitHub repo (e.g. https://GitHub.com/charms/haproxy/tree/precise && https://GitHub.com/charms/haproxy/tree/trusty or a better example https://GitHub.com/charms/haproxy/tree/centos7)  This is needed when there needs to be  two different versions of a charm.
	- As a best practice charms should endevour to have one charm per OS. When the divergence for a given charm is great enough (e.g. Ubuntu to CentOS) we should look at creating a new branch in git.

## ACLs for Charms and Blobs

### Work Items

1. Namespace that holds revisions for a charm needs to store ACLs.
1. Charm store needs to check them against API requests.
1. The API to get the resource need to have a reference to the top-level charm. TBD. So we can check the read permission.

Need to decide how we want to deal with access to metadata and content.
Should we always allow full access to all blobs and content if you can deploy,

### Option #1

r=metadata
w=publish
x=deploy

#### Public charm (0755)

|   |   | Metadata | Publish | Deploy |
|------------|----------|----------|---------|--------|
| maintainer | charmers | X | X | X |
| installers | charmers | X |   | X |
| everybody  | - | X |   | X |

#### Charm under test (0750)

|   |   | Metadata | Publish | Deploy |
|------------|----------|----------|---------|--------|
| maintainer | cmars | X | X | X |
| installers | qa | X |   | X |
| everybody  | - |   |   |   |

#### Gated charm (0754)

You can see it, but you have to get approval (added to installers).

|   |   | Metadata | Publish | Deploy |
|------------|----------|----------|---------|--------|
| maintainer | ibm | X | X | X |
| installers | ibm-customers | X |   | X |
| everybody  | - | X |   |   |

### Option #2

r=read content of charm
w=publish
x=deploy and read metadata

#### Public charm (0755)

|   |   | Content | Publish | Metadata & Deploy |
|------------|----------|----------|---------|--------|
| maintainer | charmers | X | X | X |
| installers | charmers | X |   | X |
| everybody  | - | X |   | X |

#### Charm under test (0750)

|   |   | Content | Publish | Metadata & Deploy |
|------------|----------|----------|---------|--------|
| maintainer | cmars | X | X | X |
| installers | qa | X |   | X |
| everybody  | - |   |   |   |

#### Gated charm (0710)

You can see it, but you have to get approval (added to installers).

|   |   | Content | Publish | Metadata & Deploy |
|------------|----------|----------|---------|--------|
| maintainer | ibm | X | X | X |
| installers | ibm-customers |   |   | X |
| everybody  | - |   |   |   |

#### Commercial charm with installer-inaccessable content (0711)

|   |   | Content | Publish | Metadata & Deploy |
|------------|----------|----------|---------|--------|
| maintainer | ibm | X | X | X |
| installers | ibm-customers |   |   | X |
| everybody  | - |   |   | X |

## Upgrades

Prior to 1.18, Juju did not really support upgrades. Each agent process listened to the agent-version global config value and restarted itself with a later version of its binary if required.

1.18 introduced the concept of upgrade steps, which allowed for ordered execution of business logic to perform changes associated with upgrading from X to Y to Z. 1.18 also made the machine agents on each node solely responsible for initiating an upgrade on that node, rather than all agents (machine, unit) acting independently. However, several pieces are still missing….
 
### Agenda items

- Coordination of node upgrades - lockstep upgrades
- Schema updates to database
- HA - What needs to be done to support upgrades in a HA environment?
- Read only mode to prevent model or other changes during upgrades
- How to validate an upgrade prior to committing to it, e.g. bring up shadow Juju environment on upgraded model and validate first before either committing or switching back?
- Perhaps a `--dry-run` to show what would be done?
- Authentication/authorization - restrict upgrades to privileged users?
- How to deal with failed upgrades / rollbacks? Do we need application level transactions?
- Testing of upgrades using dev release - faking reported version to allow upgrade steps to be run etc

### Work items for schema upgrade

Key assumption - database upgrades complete quickly 

1. Implement schema upgrade code (probably as an upgrade step).
	- mgo supports loading documents into maps, so we do not have to maintain legacy structs.
	- Record “schema” version.
1. Implement state/mongo locking, with explicit upgrading/locked error.
	- One form of locking is to just not allow external API connections until upgrade steps have completed, since we know we just restarted and dropped all connections.
1. Introduce retry attempts in API server around state calls.
1. Take copy of db prior to schema upgrade and copy back if it fails.
1. Upgrade steps for master state server only.
1. Coordination between master/slave state servers to allow master to finish first.

### Work items for upgrade story

- Allow users to find out what version it will pick when upgrading.
- Commands to report that upgrade is in progress if run during an upgrade.
- Peer group worker to only start after an upgrade has completed.
- Update machine status during upgrade, set error status on failure.

## Juju Integration with Oasis TOSCA standards (IBM)

[TOSCA](https://www.oasis-open.org/committees/tc_home.php?wg_abbrev=tosca) is a standard aimed at, “Enhancing the portability and management of cloud applications and services across their lifecycle.” In discussions with IBM we need to integrate Juju into TOSCA standards as part of our agreement. Thus we need to define the following:

- [TOSCA](https://www.oasis-open.org/committees/tc_home.php?wg_abbrev=tosca) - simple profile yaml doc, updated approx weekly
- Discuss who will lead this effort and engage with IBM.
- Define the correct integration points.
- Define design and architecture of TOSCA integration.
- Define what squad will deliver the work and timelines.

### Goal

- Drag a TOSCA spec onto the juju-gui and have the deployment happen.

## Other OS Workloads

Juju has been Ubuntu only so far but never intended to be only Ubuntu. We were waiting for user demand. It seems some of that demand has now happened.  From earlier discussions the following areas have been identified for work:

1. Remove assumptions about the presence of apt from core
1. Deal with upstart vs systemv vs windows services init system differences for agents
1. Deal with rsyslog configuration
1. Define initial charms (bare minimum would be ubuntu charm equivalents)
1. Update cloud-init stuff for alternate OS
1. SSH Configuration
1. Define and handle non Ubuntu images

Key questions are:

1. Which is going to be first
	- expect the windows workloads as that has been implemented already and we just need to integrate
1. How important is this compared to the other priorities?

I don’t think there are any questions around “should we do it”, just “when should we do it”.

### CentOS / SLES

Hopefully we can handle both CentOS and SLES at one go as they are based on very similar systems.  We may need to abstract out some parts, but on the whole, they *should* be very similar.  Again there should be a lot of overlap between Ubuntu and both CoreOS and SLES, with obvious differences in agent startup management and software installation. The writing of the actual charms are outside the scope of this work, although we should probably make CentOS and SLES charms to mirror the ubuntu charm that just bring up an appropriate machine.

### Windows

We have work that has been done already by a third party to get Juju working to deploy windows workloads. It is expected that this work that is done will not either cleanly merge with current trunk, nor necessarily meet our normal demands of tests, robustness or code quality.  We won’t really know until we see the code.  However what it does give us is something that works that clearly identifies all of the Ubuntu specific parts of the codebase, and will give us a good foundation to work from to get the workload platform agnostic nature we desire.

### Notes

- Need to get code drop from MS guys.
- Use above to identify non Ubuntu specific parts of code.
- We do interface design, CentOS implementation.
- We hand the above back to MS guys and they use that as template to re-do the Windows version.
- Excludes state server running on Windows.
- Manual provisioning Windows instances.
- Local provider (virtual box) on Windows.

## 3rd Party Provider Implementations

- Improving our documentation around what it takes to implement a Provider.
- We still call them Environ internally.

## Container Addressability (Network Worker)

- [Earlier notes on Networking](https://docs.google.com/a/canonical.com/document/d/1Gu422BMAJDohIXqm6Vq4WTrtBV8hoFTTdXvXDQCs0Gs/edit)
- Link to [Juju Networking Part 1](https://docs.google.com/a/canonical.com/document/d/1UzJosV7M3hjRaro3ot7iPXFF9jGe2Rym4lJkeO90-Uo/edit#heading=h.a92u8jdqcrto) early notes
-What are the concrete steps towards getting containers addressable on clouds?
-  Common
	- Allocate an IP address for the container (provider specific).
	- Change the NI that is being used to be bridged.
	- Bring up the container on that bridged network and assign the local address.
- EC2
	- **ACTION(spike)**: How do we get IP addresses allocated in VPC?
	- Anything left to be done in goamz?
- OpenStack
	- Neutron support in lp:goose.
		- Add neutron package.
		- Sane fallback when endpoints are not available in keystone (detect if Neutron endpoints are supported or not and if not report the error).
		- New mock implementation (testservers).
		- Specify ports/subnets at StartInstance time (possibly a spike as well).
		- Add/remove subnets.
		- Add/remove/associate ports (Neutron concept, similar to a NIC).
		- Add/remove/relate bridges? Probably not needed for now.
		- Maybe security groups via Neutron rather than Nova.
	- Potential custom setup once port is attached on machine

We need a Networker worker at the machine level to manage networks. What about public addresses? We want `juju expose` to grow some ability to manage public addresses. Need to be aware that there’s a limit of 5 elastic IPs per region per account. Can instead get a public address assigned on machine startup that cannot be freely reassociated. Need to make a choice about default VPC vs creating a VPC. Using only default VPC is simpler.

### Potentially out of scope for now

- Using non-default VPC - requires several additional setup steps for routes and such like.
- Networking on providers other than EC2/OpenStack, beyond making sure we don’t bork on interesting setups like Azure.
- Networking on cloud deployments that do not support Neutron (e.g. HP).

Separate discussion: Update ports model to include ranges and similar.

Switching to new networking model also enables much more restrictive firewalling, but does require some charm changes. If charms start declaring ports exposed on a private networks, it would be possible to skip address-per-machine for non-clashing ports. Also allows more restrictive internal network rules.

### Rough Work Items

1. When adding a container to an existing machine, Environment Provisioner requests a new IP address for the machine, and records that address as belonging to the container.
1. `InstancePoller` needs to be updated, so that when it lists the addresses available for a machine, it is able to preserve the allocation of some addresses to the hosted containers.
1. `Networker` worker needs to be able to set up bridging on the primary instance network interface, and do the necessary ebtables/iptables rules to use the same bridge for LXC containers (e.g. any container can use one of the host instance’s allocated secondary IP addresses so it appears like another instance on the same subnet).
1. Existing MaaS cloudinit setup for VLANs will be moved inside the networker worker.
1. Networker watches machine network interfaces and brings them up/down as needed (e.g. doing dynamically what MaaS VLAN cloudinit scripts do now and more).

## Leader Elections

Some charms need to elect a “master” unit that coordinates activity on the service.   Also, Actions will at times need to be run only on the master unit of a service.  

- How do we choose a leader?
- How do we read/write who the leader is?
- How do we recover if a leader fails?
- The current leader can relinquish leadership (e.g. this is a round robin use case).

Lease on leader status.  Allows caching, prevents isolated leader from performing bad actions.  If leader is running an action and can’t renew lease, must kill action.  Same with hooks that require leader.  Agent controls leader status, does the killing.

## Improving charm developer experience

Charms are the most important part of Juju.  Without charms people want to use, Juju is useless. We need to make it as easy as possible for developers outside Canonical to write charms.

Areas for improvement:

- Make charm writing  easier.
- Make testing easier.
- Make charm submission painless.
- Make charm maintenance easier.
- What are the current biggest pain points?

## Juju needs a distributed log file

We currently are working on replicating rsyslog to all state servers when in HA.  Per Mark Ramm, this is good enough for now.  We may want to discuss  a real distributed logging framework to help with observability, maintenance, etc.

### Notes

- Kapil says Logstash or Heka. Heka is bigger and more complicated, and suggests Logstash is more likely to be suitable.
- Wayne has used Apache Scribe in the past.
- Requirements:
	- Replicated (consistently) across all state servers.
	- Newly added state servers must have old log messages available.
	- Must be tolerant of state server failures.
	- Store and forward.
	- Nice to have: efficient querying.
	- Nice to have: surrounding tooling for visualization, post-hoc analysis, …
	- Encrypted log traffic.

### Actions

Juju actions are charm-defined functionality that is user-initiated and take parameters  and are executed on units. Such as backing up mysql.

### Open Questions

- How do we handle history and results?
- How do we handle actions that require leaders on services with no leaders?
- Is there anything else controversial in the spec?
- Do we have a piece of configuration on the action defining what states it's valid to run it in?
- Users should be made away of the lifecycle of an action. For example, what unit is currently backing up, the progress of the backup and the resolution of the backup if it was successful or not.

Actions have

1. State
1. Lifecycle
1. Reporting

Actions accept parameters.
Actions directory at the top level: Contents a bunch of named executables.
`actions.yaml` has a key for each action.
E.g. service or unit action, e.g. schema for the parameters.  (JSON schema expressed in YAML).

There are both unit-level and service-level actions.  Unit-level will be done first.

Collections of requests and results.
Each unit watches the actions collection for actions targeted to itself.
Not notified of things they don't care about.
When you create an action, you get a token, you watch for the token in the results table.
Non-zero means failure.  Error return from an action doesn't put the unit into an error state.

Actions need to work in more places than hooks.  We don't want to run them before start or after stop.  We want to run them while in an error state.

```
$ juju do action-name [unit-or-service-name] --config path/to/yaml.yml
```

By specifying a service name for a unit action, run against all units by default.

Results are yaml.

stdout -> log

Hook and action queues are distinct

### Work Items

1. Charm changes:
	- Actions directory (like hooks, named executables).
	- Top-level actions.yaml (top-level key is actions, sub-keys include parameters, description).
1. State / API server:
	- Add action request collection.
	- Add action result collection.
	- APIs for putting to action/result collections.
	- APIs for watching what request are relevant for a given unit.
	- APIs for watching results coming in (probably filtered by what unit/units we're interested in).
	- APIs for listing and getting individual results by token.
	- APIs for getting the next queued action.
1. Unit agent work:
	- Unit agent's "filter" must be extended to watch for relevant actions and deliver them to the uniter.
	- Various modes of the uniter need to watch that channel and invoke the actions.
	- Handwavy work around the hook context to make it capable of running actions and persisting results.
	- Hook tools:
		- Extract parameters from request.
		- Dump results back to database.
		- Error reporting.
		- Determine unit state?
1. CLI work:
	- CLI needs a way to watch for results.
	- juju do sync mode
	- juju do async mode
	- juju run becomes trivially implementable as an action
1. API for listing action history.
1. Leader should be able to run actions on its peers (use case: rolling upgrades).
1. Later: Fix up the schema for charm config to match actions.

## Actions, Triggers and Status

What are triggers? (related to Actions, IIRC)

### Potential applications

- Less polling for UI, deployer, etc.

### Topics to discuss

- Authentication
- Filtering & other features
- API
- Implementation

## Combine Unit agent and Machine agent into a single process

- What is the expected benefit?
	- Less moving parts, machine and unit agents upgrade at the same time.
	- Avoids N unit agents for N charms + subordinates (when hulk-smashing for example).
	- Less deployment size footprint (one less jujud binary).
	- Less workers to run, less API connections.
- What is the expected cost?
	- rsyslog tagging (logs from the UA arrive with the agent’s tag; we need to keep that for observability).
	- Concrete steps to make the changes.

Issues with image based deployments?

- No issues expected.
- Even we need a juju component inside the container, no issue.

### Work Items

1. Move relevant unit agent jobs into machine agent (drop duplicates).
1. Remove redundant upgrade code.
1. Change deployer to start new uniter worker inside single agent.
1. Change logging (loggo/rsyslog worker) to allow tags to be specified when logging so that each unit still logs with its own tag.
1. (Eventually) consolidate previously separate unit/machine agent directories into single dir.
1. ensure juju-run works as before

## Backup/Restore

- Making current state work:
	- We need to have the mongo client for restore.
	- We need to ignore replicaset.
- What will it take to implement a “proper” backup, instead of just having some scripts that mostly seemed to work one time.
	- Back-up is an API call
	- Restore should grow in `jujud`.
		- Add a restore to the level of bootstrap?
- Turning our existing juju-backup plugin from being a plugin into being integrated core functionality.
	- Can we snapshot the database without stopping it?
	- How will this interact with HA? We should be able to ask a secondary to save the data.
	- It is possible to mongodump a running process, did we consider that rather than shutting mongo down each time?
	- Since we now always use --replicaSet even when we have only 1, what if we just always created a “for-backup” replica that exists on machine-0. Potentially brought up on demand, brought up to date, and then used for backup sync.
- juju-restore
	- What are the assumptions we can reliably make about the system under restore?
		- E.g., in theory we can assume all members of the replica are dead, otherwise you wouldn’t be using restore, you would just be calling enusre-availability again.
	- Can we spec out what could be done if the backup is “old” relative to the current environment? Likely most of this is “restore 3.0” but we could at least consider how to get agents to register their information with a new master.

### Concrete Work Items

1. Backup as a new Facade for client operations.
1. `Backup.Backup` as an API call which does the backup and stages the backup content on server disk. API returns a URL that can be used to fetch the actual content.
1. `Backup.ListBackups` to get the list of tarballs on disk.
1. `Backup.DeleteBackups` to clean out a list of tarballs.
1. HTTP Mux for fetching backup content.
1. Juju CLI for
	- `juju backup` (request a backup, fetch the backup locally)

## Consumer relation hooks run before provider relation hooks

[Bug 1300187](https://bugs.launchpad.net/juju-core/+bug/1300187)

- IIRC, William had a patch which made the code prefer to run the provider side of hooks first, but did not actually enforce it strictly. Does that help, or are charms still going to need to do all the same work.
- Does it at least raise the frequency with which charms “Just Work” or does it make it hard to diagnose when they “Just Fail”.

## Using Cloud Metadata to describe Instance Types

We currently hard-code EC2 instance types in big maps inside of juju-core. When EC2 changes prices, or introduces a new type, we have to recompile juju-core to support it. Instead, we should be able to read the information from some other source (such as published on streams.canonical.com since AMZ doesn’t seem to publish easily consumable data).

- OpenStack provider already reads the data out of keystone, are we sure AMZ doesn’t provide this somewhere.
- Define a URL that we could read, and a process for keeping it updated.

### Work Items

1. Investigate the instance type information each cloud type has available - both programmatically and elsewhere.
1. Define abstraction for retrieving this information. Some clouds will offer this information directly, others will need to get this from simplestreams. Some cloud types may involve getting the information from mixed sources.
1. Support search path for locating instance information and mixed sources.
1. Ensure process for updating Canonical hosted information is in place.
1. Document how to update instance type information for all cloud types.
1. API for listing instance types (for GUI).

## API Versioning

We’ve wanted to add this for a long time.

- Possible [spec](https://docs.google.com/a/canonical.com/document/d/1guHaRMcEjin5S2hfQYS22e22dgzoI3ka24lDJOTDTAk/edit#heading=h.avfqvqaaprn0) for refactoring API into many Facades
- [14.04 Spec](https://docs.google.com/a/canonical.com/document/d/12SFO23hkx4sTD8he61Y47_kBJ3H5bF2KOwrFFU_Os9M/edit)
- Can we do it and remain 2.x compatible for the lifetime of Trusty?
- Concrete design around what it will look like.
	- From an APIServer perspective (how do we expose multiple versions).
	- From an API Client perspective.
	- From the Juju code itself (how does it notice it wants version X but can only get Y so it needs to go into compatibility mode, is this fine grained on a single API call, or is this coarse grained around the whole API, or middle ground of a Facade).

### Discussion

- We can use the string we pass in now ("") to each Facade, and start passing in a version number.
- Login can return the list of known Facades and what version ranges are supported for each Facade.
- Login could also start returning the environment UUID that you are currently connected to.
- With that information, each client-side Facade tracks the best version it can use, which it then passes into all `Call()` methods.
- Compatibility code uses `Facade.CurrentVersion()` to do an if/then/switch based on active version and do whatever compatibility code is necessary.

### Alternatives

- Login doesn’t return the versions, but instead when you do a `Call(Facade, VX)` it can return an error that indicates what actual versions are available.
	- Avoids changing  Login.
	- Adds a round-trip whenever you are actually in compatibility mode.
	- Creates clumsy code around: `if Facade.Version < X { do compat} else { err : =tryLatest; if err == IsTooOld {compat}}`
- Login sets a global version for all facades.
	- Seems a bit to coarse grained that any change to any api requires a global version bump (version number churn).
- Each actual API is individually versioned.
	- Seems to fine grained, and makes it difficult to figure out what version needs to be passed when (and then deciding when you need to go into compat mode).

## Tech-debt around creating new api clients from Facades

[Bug 1300637](https://bugs.launchpad.net/juju-core/+bug/1300637)

- Server side [spec](https://docs.google.com/a/canonical.com/document/d/1guHaRMcEjin5S2hfQYS22e22dgzoI3ka24lDJOTDTAk/edit).
- We talked about wanting to split up Client into multiple Facades. How do we get there, what does the client-side code look like
- We originally had just `NewAPIClientFromName`, and Client was a giant Facade with all functions available
- We tried to break up the one-big-facade into a few smaller ones that would let us cluster functionality and make it clearer what things belonged together. (`NewKeyManagerClient`).
- There was pushback on the proliferation of lots of New*Client functions. One option is that everything starts from `NewAPIClientFromName()`, which then gets a `NewKeyManager(apiclient)`. 

## Cross Environment Relations

We’ve talked a few times about the desirability of being able to reason about a service that is “over there”, managed in some other environment.

- Last [spec](https://docs.google.com/a/canonical.com/document/d/1PpaYWvVwdF55-pvamGwGP23_vHrmFwCW8Bi-4VUg-u4/edit)
	- Describes the use cases, confirm that they are still valid.
- We should update to include the actual user-level commands that would be executed and what artifacts we would expect (e.g., `juju expose-service-relation` creates a `.jenv/.dat/.???` that can be used with `juju add-relation --from XXX.dat`).

### Notes

Expose endpoint in env 1, this generates a jenv (authentication info for env1) that you can import-endpoint into another environment.  This has env2 connects to env1, asks for information about the service in env1.  This creates a ghost service in env2 that exposes a single endpoint, which is only available for connecting relations (no config editing etc).  There is a continuous connection between the two environments to watch whether the service goes down, etc.  
Propagate IP changes to other environment.  Note that it is currently broken for relations even in a single environment.
Cross environment relations always use public addresses (at least to start).
Note that the ghost service name may be the same as an existing service name, and we have to ensure that’s ok.

## Identity & Role-Based Access Controls

- [Juju Identity, Roles & Permissions](https://docs.google.com/a/canonical.com/document/d/138qGujBr5MdxzdrBoNbvYekkZkKuA3DmHurRVgbTxYw/edit#heading=h.7dwo7p4tb3gm)
- [Establishing User Identity](https://docs.google.com/a/canonical.com/document/d/150GEG_mDnWf6QTMc1kBvw_x_Y_whGVN19mr3Ocv6ELg/edit#heading=h.aza0s6fmxfs9)

### Current Status

- Concept of service ownership in core.
- Add/remove user, add-environment framework done, not exposed in CLI.

What does a minimum viable multi-user Juju look like? (Just in terms of ownership, not ACLs).

- `add-user`
- `remove-user`
- `add-environment`
- `whoami`

### 14.07 (3mo)

- Beginnings of role-based access controls on users (Implementation of RBAC in core is another topic).
- [Juju Identity, Roles & Permissions](https://docs.google.com/a/canonical.com/document/d/138qGujBr5MdxzdrBoNbvYekkZkKuA3DmHurRVgbTxYw/edit#heading=h.7dwo7p4tb3gm).
- Non-superusers: read-only access at a minimum.

### 14.10 (6mo)

- Command-line & GUI identity provider integrations.

### 15.01 (9mo)

- IaaS, mutually-trusted identities across enterprises.
- Need a way to securely broker B2B IaaS-like transactions.

## Iron Clad Test Suite

The Juju unit test suite is beset by intermittent failures, caused by a number of issues:

- Mongo and/or replica set related races.
- Access to external URLs e.g. charm store.
- Isolation issues such that one failure cascades to cause other tests to fail.

There are also other systemic implementation issues which cause fragility, code duplication, and maintainability problems:

- Lack of fixtures to set up tools and metadata (possibly charms?).
- Code duplication due to lack of fixtures.
- Issues with defining tools/version series such that tests and/or Juju itself can fail when run on Ubuntu with different series.

Related but not a reliability issue is the speed at which the tests run e.g. the Joyent tests take up to 10 minutes. We also have tests which were set up to run against live cloud deployments but which in practice are never run - we now rely on CI. 

Over the last cycle, things have improved, and there are certain issues external to Juju (like mongo) which contribute to the problems. But we are not there yet and must absolutely get to the stage where tests pass first time, every time on the bot and when run locally. We need to consider/discuss/agree on:

- Identify current failure modes.
- Harden test suite to deal with external failures, fix juju-core issues.
- Introduce fixtures for things like tools and metadata setup and refactor duplicate code and set up.
- Document fixtures and other test best practices.

### Work Items - Core - Refactoring and Hardening

Juju does what it is supposed to do, but has a number of rough edges when it comes to various non-functional requirements which contribute to the fact that often Juju doesn’t Just Work, and many times requires an unacceptably high level of user expertise to get things right. These non-functional issues can very broadly be classified as:

- **Robustness** - Juju needs to get better at dealing with underlying issues, whether transient network related, provider/cloud related, or user input. 
- **Observability** - Juju needs to be less of a black box, and expose more of what’s going on under the covers, so that both humans and machine alike can make informed decisions in response to errors and system status.
- **Usability** - Juju needs to provide a UI and workflow that makes it difficult to make mistakes in the first place; to catch and report errors early as close to the source as possible.

As well as changes to the code itself, we should consider process changes which will guide how new features are implemented and rolled out. There is currently a disconnect between developers and users (real world). A developer will often test a new feature in isolation on a single cloud which works first time, deployed on an environment with a few.

- Rename `LoggingSuite` to something else, make the default base suite with mocked out `$HOME`, etc.
	- Identify independent fixtures (e.g. fake home, fake networking, …), and compose base suite from them.
	- Create fake networking fixture that replaces the default HTTP client with something that rejects attempts to connect to non-localhost addresses.
	- Update tools fixture and related tests.
- Introduce in-memory mock mgo for testing independent of real mongo server.
- Continue separation of api/apiserver in unit tests to enable better error checking.
- Document current testing practices to avoid cargo culting of old practices, ensure document is kept up-to-date at code review time.
- Update, speed-up Joyent tests (and all tests in general). Joyent tests currently take ~10mins, which far too long.
- Suppress detailed simplestreams logging by default in (new) ToolsSuite by setting streams package logging level to INFO suite setup.
- Delete live tests from juju-core.

Nodes at best. They won’t be exposed to the pain associated with / needed to diagnose and rectify faults etc since it’s often easier to destroy-environment and start again, or a new revision will have landed and CI will start all over again. More often than not, it’s the QA team who has to diagnose CI failures which are raised as bugs but with developers being spared the pain of the root cause analysis and any fixes often addressing a specific bug rather than a systemic, underlying issue.

### Items to consider

- Architectural layers - what class of error should each layer handle and how should errors be propagated / handled upwards.
- How to expose/wrap provider specific knowledge to core infrastructure so that such knowledge can be used to advantage?
- Where’s the line between Juju responding to issues encountered vs informing and immediate feedback of problems but CI issues lack immediate visibility.
- Close the loop between real world deployment and developers.
- How to ensure teams take ownership of non-functional issues?
- Tooling - targeted inspection of errors, decisions made by Juju, e.g. utilities exist to print where tools/image metadata comes from; is that sufficient, what else is needed?
- Roadmap would be awesome to know what features to look for in upcoming releases (and waiting for user input.
- Feature development - involve stakeholders/users (CTS?) more, at prototype stage and during functional testing?
- Hhow best to expose developers to real world, so that necessary hardening work becomes as much of an itch scratch as it does a development chore.
-C close the loop between CI and development - unit tests / landing bot provide flag specific features for additional functional testing).

### Notes

- Mock workflow in a spec/doc, quick few paragraphs about a change or feature will look for a user facing standpoint.
- Not all features require functional / UAT because of time constraints but still want to give CTS etc input to dev.
- Wishlist: Send more developers out on customer sites to get real world experiences.
- Much more involvement with IS as a customer.
- More core devs need to write charms.
- Debug log too spammy - but new incl/excl filters may help.
- Debug hooks used a lot - considered powerful tool.
- Debug hooks should be able to drop a user into a hook context when not in error state, e.g.  `juju debug hooks unit/0 config-changed`.
- Need more output in status to expose internals (Is my environment idle or busy?).
- More immediate reporting to user of charm output as deploy happens, don’t want to wait 15 minutes to see final status.
- Juju diagnose - post mortem tools <- already done via juju ready/unready, output vars etc

### Work Items

[Juju Fixes](https://docs.google.com/a/canonical.com/spreadsheet/ccc?key=0AoQnpJ43nBkJdHhnV05NcmQ3Tm5yRnIwcTlYMTZEaEE&usp=sharing)

1. Design error propagation mechanism to be used across providers.
1. Destroy Service --Force.
1. Dry run tell user what version upgrade-juju will use.
1. Inspect Relation Data.
1. Address changes must propagate to relations.
1. Use Security Group Per Service.
1. Use Instance names/tags for machines.
1. safe-provisioning-mode default.
1. Bulk machine creation.
1. Unit Ids must be unique.

## Retry on API Failures

Really part of hardening. There are transient provider failures due to issues like exceeding allowable API invocation rate limits. Currently Juju will fail when such errors are encountered and consider the errors permanent, when it could retry and be successful next time. The OpenStack provider does this to a limited extent. A large part of the problem is that Juju is chatty and makes many individual API calls to the cloud. We currently have a facility to allow provisioning to be manually retried but need something more universal and automated.

### Discussion Points

- Understanding what types of operation can produce transient errors. is it the same for all providers? what extra information is available to help with retry decision?
- Common error class to encapsulate transient errors.
- Algorithm to back off and retry.
- To what extent can Juju design / implementation change to mitigate most common cause which is exceeding rate limits.
- How to report / display retry status.
- Manual intervention still required?

### Work Items

1. Identify for for each provider which errors can be retried.
1. Juju should handle retries.
1. Above discussion points constitute the other work items.
1. Audit juju to identify api optimisation opportunities.

## Audit logs in Juju-core

The GUI needs to be able to query *something* for a persistent log of changes in the environment.

- What events are auditable ? hatch: only events that cause changes in the environment.
- tTm: who changed something, what was changed, when was it changed, what was it changed from, and to, why they were allowed to do it (Will).
- Hatch: it needs to be structured events, user, event, description, etc, NOT just a blob of text.
- Voidspace: do we need a query api on top of this ? filter by machine, by user, by operation, etc
- Audit log entries are not protected at a per row level. Viewing the audit log will require a specific permission.
- Not all users of the GUI may be able to access the audit log.
- Audit log entries may be truncated, truncation will require a high level of permissions.
- ACTION: determine auditable events.
- ACTION: determine where to store this data, and what events to audit.
- Hatch: it doesn’t need to be streaming from the start, but it should be possible.

### Work Items

1. Create a state API for writing to the audit log (in mongodb).
1. Record attempt before API request is run.
1. Record success/error after API request is run.

## Staging uncommited changes

Hatch doesn’t want to do this in Javascript, because it is not web scale. He wants the API server to handling this staging.

Thumper says that SABDFL says they want to be able to do this on the CLI as well.

- Nate: if we need to allow this to work across GUI and CLI then we have to store this data in the state.
- Nate: do we need N staging areas per environment ? Nate: No, that is crazy talk, just one per environment.
- Thumper: then we’ll need a watcher.
- ACTION: uncommitted changes are stored in the state as a single document, a big json blob
- ACTION: we need a watcher on this document.
- Voidspace: entries are appended to this document, this could lead to confusion if people are concurrently requesting unstaged changes.
- Hazmat doesn’t think we should store this in the state.
- ACTION: Mark Ramm/hazmat to talk to SABDFL about the difficulty of implementing this.
- All: do we have to have a lock or mode to enable/disable staging mode ?
- Hatch: now the GUI and the CLI have different stories, the former works in staging mode by default, and the latter always commits changes immediately.
- ACTION: a change via the CLI would error if there are pending changes, you can the push changes into the log of work with a --stage flag. Ramm: alternative, we tell the customer that the change has been staged, and they will need to ‘commit’ changes.
- ACTION: the CLI needs a ‘commit’ subcommand.
- Undo is out of scope, but permissible in a future scope; tread carefully.

### Discussion Thurs May 1

- Moved into the idea of having an ApplyDelta API that lets you build up a bunch of actions to be changed
- These actions can then all be in pending state, and you do a final call to apply them.
- The actual internal record of the actions to apply is actually a graph based on dependencies
- This lets you “pick one” to apply without applying the rest of the delta
- Internally, we would change the current API to act via “create delta, apply delta” operations.
- When a delta is pending, calling the current API could act on the fact that there are pending operations.
- Spelling is undefined, e.g.
	- `named := CreateDelta()`
	- `AddToDelta(named, operation)`
	- `ApplyDelta(named)`
	- `ApplyDelta(operations)`
- If it is just the ability to apply a listed set of operations, we haven’t actually exposed a way to collaborate on defining those operations.

## Observability

How to expose more of what Juju is doing to allow users to make informed decisions. Key interface point via `juju status`. Consider instance / unit,  observability and transparency. e.g.. what does pending really mean? Is it still in provisioning at the provider layer, is machine agent running? Is the install hook running? Is the start hook running? We collapse all of that done to a single state. we should ideally just push the currently executing hook into status.

### To discuss

- How to display error condition concisely but allowing for more information if required.
- Insight into logs - is debug log enough? (now has filtering etc).
- Feedback when running commands via CLI - often warnings are logged server side, how to expose to users; use of separate back channel?
- Interactive commands? Get input to continue or try again or error/warning?
- Consistency in logging - guidelines for verbosity levels, logging API calls etc
- How to discover valid vocabularies for machine names, instance types etc?
- How to inspect relation data?
- Should output variables be recorded/logged?
- Provide --dry-run option to see what Juju would do on upgrades.
- Better insight into hook firing.
- Ability to probe charms for health? (incl e.g. low disk space etc).
- Event driven feedback.
- Integration with SNMP systems? How to alert when issues arise?

### Work Items

- `juju status <entity>` reveals more about that entity - get all output on context that is specified.
- Add new unit state - healthy/unhealthy.
- Instance names/tag for machines (workload that caused it deployed).
- Specifically, when deploying a service or adding a unit that requires a machine to be added, the provisioner should be passed through a tag of the service name or similar to annotate the machine with on creation.
- Inspect relation data.
- implement output variables (needs spec).
- `add-machine`, `add-unit` etc need to report what was added etc
- API for vocab (inst type).

## Usability

### Covers a number of key points

- Discoverable - features should be easily discoverable via `juju help` etc.
- Validate inputs - Juju should not accept input that causes breakage, and should fail early.
- Error response - Juju should report errors with enough information to allow the user to determine the cause, and ideally should suggest a solution.
- Key workflows should be coherent and concise.
- Tooling / API support for key workflows.

### Agenda

- Identify key points of interaction - bootstrap, service deployment etc.
- Current pain points e.g.
	- Tools packaging for bootstrap for dev versions or private clouds?
	- Open close port range?
	- Security groups!
	- What else?
- What’s missing? Tooling? The right APIs? Documentation? Training?
- Frequency of pain points vs impact.

### Concrete Work Items

1. Improve `juju help` to provide pointers to extra commands.
1. Transactional config changes.
1. Fix destroy bug (destroy must be run several times to work).
	- Find or file bug on lp
1. When a machine fails, machine state in juju status displays error status with error reason.
1. Document rationale in code comment.
1. `juju destroy service --force`
1. Range syntax for open/close ports.
1. Safe mode provisioning  becomes default.
1. Garbage collect security groups.

## Separation of business objects from persistence model

A widely accepted architectural model for service oriented applications has layers for:

- services
- domain model
- persistence

The domain model has entities which encapsulate the state of the key business abstractions e.g. service, unit, machine, charm etc. This is runtime state. The persistence layer models how entities from the domain model are save/retried to/from non-volatile storage - mongo, postgres etc. The persistence layer translates business concepts like queries and state representation to storage specific concepts. This separation is important in order to provide database independence but more importantly to stop layering violations and promote correct design and separations of concerns.

### To discuss

- Break up of state package.
- How to define and model business queries.
- How to implement translation of domain <> persistence model.

### Goals

- No mongo in business objects - database agnosticism.
- Remove layering violations which lead to suboptimal model design.
- Scalability via ability to implement pub/sub infrastructure on top of business model rather than persistence model; no more suckiSpng on mongo firehose.

### Work Items

1. Spike to refactor a subset of the domain model (e.g. machines). 
1. Define and use patterns (e.g. “named query”) to abstract out database access further (in spike).
1. Define and use patterns for mapping/transforming domain objects to persistence model.
1. If possible, define and implement integration with pub/sub for change notification.

## Juju Adoption Blockers

[Slides with talking points](https://docs.google.com/a/canonical.com/presentation/d/1jcJ93Npuo60Iyy0BGSNap1kekQNxiZ7rDBJfuxAv_Go/edit#slide=id.ge4adadaf_1_645)

## Partnerships and Customer Engagement

- Juju GUI has been a tremendous help.
	- Sales team enabler, to quickly and easily show Juju.
- Every customer/partner asks
	- Where can I get a list of all charms?
	- Where can I get a list of all available relations?
	- Where can I get a list of all available bundles?
	- Where can I get a list of all supported cloud providers?
	- What about HA?  What happens if the bootstrap node goes away?
		- We need to start demonstrating this, ASAP!
	- What if one of the connected services goes away?  What does Juju do?
	- So, great, I can use Juju to relate Nagios and monitor my service.  But what does Juju do with that information?  Can’t Juju tell if a service disappears?
	- Auto-scaling?  Built in scalability is great, but manually increasing units is only so valuable.
	- What do you mean, there aren’t charms available for 14.04 LTS yet?
	- *Yada yada yada* Docker *yada yada yada*?
- Our attempts to shift the burden of writing charms onto partners/customers have yielded minimal results.
- Pivotal/Altoros around CloudFoundry
	- CloudFoundry is so complicated, Pivotal developed their own custom Juju-like tool (BOSH) to deploy it, and their own “artifact” based alternative to traditional Debian/Ubuntu packaging.
	- CloudFoundry charms (and bundles) have proven a bit too complex for newbie/novice charmers at Altoros to develop, at the pace and quality we require.

## Juju 2.0 Config

- Define providers and accounts as a first class citizen.
- Eventually remove environments.yaml in favor of the above account configuration and .jenv files
- Change ‘juju bootstrap’ to take an account and --config=file/--option=”foo=var” for additional options.
- `juju.conf` needs
	- simplestreams source for provider definitions, defaulting to https://streams.canonical.com/juju/providers.
		- A new stream type “providers” containing the environment descriptions for known clouds (e.g. hpcloud has auth_url:xyz, type:OpenStack, regions-available: a,b,c, default-region:a).
		- Juju itself no longer includes the information inside the ‘juju’ binary, but depends on that information from elsewhere.
	- Providers section.
		- Locally define the data that would otherwise come from above.
	- Accounts section.
		- Each account references a single provider.
		- Local overrides for environment details (overriding defaults set in provider).

## Distributing juju-core in Ubuntu

Landscape has a stable release exception for their client, not a micro release exception. We fulfil the rules for this even better than landscape does, as we have basically no dependencies at all.

We can split juju the client from jujud the server, though this isn’t terribly useful for us outside of making distro people happy.

Landscape process has two reviews before code lands, we used to do this but changed. Didn’t seem to drop quality our end.

Could raise at a tech board meeting item to sort out stable release things.

Having to have separate source packages for client and server would be annoying but painful, could we have different policies for binary packages generated from the same source package?

Dynamic linking gripes are not imminently going to be solved by anyone.

Have meeting with foundations to resolve some unhappinesses.

## Developer Documentation

- https://juju.ubuntu.com/dev/ - Developer Documentation.
- There exists an automated process to pull the files from the doc directory in the juju-core source tree and process the markdown into html, and uploads it into the WordPress site.
- Minimal topics needed
	- Architecture overview
	- API overview
	- Writing new API calls
	- What is in state (our persistent store - horrible name, I know)?
	- How the mgo transactions work?
	- How to write tests?
		- Base suites
		- Environment isolation
		- Patch variables and environment
		- Using gocheck (filter and verbose)
		- Table based tests vs. simple tests
		- Test should be small and obviously correct
	- Developer environment setup
	- How to run the tests?
	- `juju test <filter> --no-log (plugin)`
- https://juju.ubuntu.com/install/ should say install juju-local

## Tools, where are they stored, sync-tools vs bootstrap --source

- FindTools is called whenever tools are required, which searches all tools sources again.
- When tools are located in the search path, they are copied to env storage and accessed from there when needed.
- Find is only to be called once at well defined points : bootstrap and upgrade. the tools are fetched into env storage so that e.g. during upgrade tools are sourced from there.
- Need tools catalog separate from simplestreams for locating tools in env storage.
- Bootstrap and upgrade and sync-tools need --source.

As is the case now, if --source is not specified, an implicit upload-tools will be done.

## Status - Summary vs Detailed

Status is spammy even on smallish environments.  It’s completely unusable on mid sized and larger environments. Can we make it easier to read, or make another status that is more of a summary view?

### Work Items

1. Identify items in status output that may break people’s scripts if changed or removed.
1. Add flags: 
	- `--verbose/-v`: total status, current output + HA + networking junk
	- `--summary`: human readable summary - not YAML (this is dependant on mini-plugin below)
	- “`--interesting`”: items that aren’t “normal” (e.g. agent state != “Started”)
1. Write mini-plugin that takes human readable YAML and generates human readable output e.g. HTML.
1. Use watcher to monitor status instead of polling juju status cmd.
1. Extend filtering.

## Relation Config

When adding a relation, we want to be able to specify  configuration specific to that relation. In settings terms, this will be “service-relation-settings”. We need to set config for either end of the service. Settings data stored for relation as a whole.

The relation config schema is defined in charm’s `metadata.yaml`. Separate config for each end of the relation.

The config is specified using add-relation `config.yaml` via `--config` option.

New Juju command `relation-get-config [-r foo]` to get config from local side of the relation. If inside hook we don’t need -r.

New `juju set-relation config.yaml` which will cause relation-config-changed hook to run.

### Work Items

1. New add relation `metadata.yaml` schema.
1. Ability to store relation settings in mongo.
1. Support for processing relation config in `add-relation`.
1. `relation-get-config` command.
1. `set-relation-config` command.
1. `relation-config-changed` hook

## Bulk Cloud API

The APIs we use to talk to cloud providers are too chatty e.g. individual calls to start machines, open individual ports.

When starting many instances, partition them into instances with same series/constraints/distribution group and ask provider to start each batch.

### Work Items

1. Unfuck instance broker interfaces to allow bulk invocation.
1. Rework provisioner.
1. Change instance data so that it is fully populated and not just a wrapper around an instance id, causing more api calls to be required.
1. Audit providers to identify where bulk api calls are not used.
1. Start instances to return ids only, get extra info in bulk as required.
1. Single shared instance state between environs (updated by worker).
1. Refactor prechecker etc to use cache environ state - reduce `New()` environ calls.
1. Stop using open/close ports and use iptables instead.
1. Use single security group.
1. Use firewaller interface in providers to allow azure to be handled.
1. Drop firewall modes in ec2 provider.
1. Support specifying port ranges not individual ports (e.g. charm metadata).
1. For hook tools - open ports on network for a machine not a unit.

## Tools Placement

- Allow storage of tools in the local enviroment.
- Providing a catalog of the tools in the local environment.
- Refactoring the current tools lookup to use the catalog.
- Provide tools import utility to get new tools into the environment.
- Upgrades to check tools catalog to ensure tools are available for all required series, arches etc.
- Same model as for charms in state.

## Juju Documentation

**William**: Write documentation while designing the feature, and give them to Nick etc. before writing code.  This is the word of god.

**Nate**:  Use changelog file in juju-core repo to log features and bugfixes with merge proposals.

**Nick & Jorge**:  we’re just a couple people, juju core is 20 people now.

**Ian**: can’t require changelog per merge, since a single feature may be many many merges, which might have no user facing features.

This must actually happen or Jorge has permission to kill Nate.

Nate to get buy in from team leads.

# Charm Config Schema

Users find our limited set of types in config (String, Bool, Int, Float) limited, and have to do things like pickle lists as base64. See [bug](https://bugs.launchpad.net/juju-core/+bug/1231526) which largely covers this.

- Map existing YAML charm config descriptions into a JSON schema.
- Extend existing YAML config to something that can be mapped well to JSON schema.
- Currently have a config field in charm document.
- Create a schema document that charm links to.
- Upgrade step that takes existing config field and creates new document linked to charm.
- Add support in `juju set` for new format.
- Add flag to `juju get` to output new format.

New types we want: enums, lists, maps (keys as strings, values as whatever).

Open questions: how charms upgrade their own schema types - there’s existing pain here where for instance the OpenStack charms are stuck using “String” for a boolean value because they cannot safely upgrade type.

Pyjuju had magic handling for schlurping files, there’s a bug feature request for a ‘File’ type.

Note this work does not include constraint vocabularies.  See Ian Booth for that work.

# Juju Solutions & QA

This is very dependent on which charm you are looking at.  I assume there were particular things that came up in the Cloud Foundry work that need attention.   We have been building up test infrastructure quite quickly, which is one part of helping improve quality -- but the biggest thing is growing communities around particular charms.

# Juju QA

## CABS Reporting

The feature has stalled as goals and APIs churned.

1. What are the goals of reporting?
1. What is the data format that cabs will provide for reporting?
1. How do we display the reports?

## Scorecard

The scorecard is a progress report to measure our activity and correlate it to our successes and failures. Most of the work is done by hand. Though most of the information gathering can be automated, it was the lowest priority for the Juju QA team. How much time will we save if we automate some or all of the information gathering?

Juju QA has scripted most of what it gathers for the score card. The data is entered by hand instead of added to tables and charts by an automated process. These are the kinds of data that the team knows how to gather:

1. Bugs reported, changed, or fixed.
1. Branch commits.
1. Time from report, to start to release of bugs and commits.
1. Releases of milestones.
1. Downloaded installers and release tarballs (packagers and homebrew).
1. Installs of clients from PPAs.
1. Downloads of tools from public streams.

### Work Items

1. GUI
	1. Bundles deployed
	1. Charms deployed
	1. visiting to jujucharms.com and juju.ubuntu.com
	1. Quick-start downloads
	1. Number of releases
	1. Number of bugs
	1. Number of bugs closed
1. Core
	1. Number of external contributors
	1. Number of fix committed
	1. Number running envs (charmstore is queried every 90 min for new charms)
		- Do we know which env the charm query was for?
	1. Client installs (from ppa, cloud archive trusty)
	1. Number of tools downloaded (from containers and streams.c.c)
	1. Add anonymous stat collection to juju to learn more
1. Eco
	1. Number of canonical and non-canonical charm committers
	1. Number of people in #juju (and #juju-dev)
	1. Number of subscribers juju and juju-dev mailing lists
	1. NUmber of charms audited
	1. AskUbuntu Conversion (Questions Asked & Answered)
	1. Number of tests in charms
1. QA
	1. Metric
	1. Days to bug triage
	1. CI tests run per week
	1. Number of solutions tested
	1. Number of clouds solution tested on
	1. Number of juju core releases

## Charm Testing Reporting

Charm test reporting has faced obstructions from several causes. There are two central issues. One, reliable delivery of data to report, and two, completion of the reporting views.

1. Charm testing data formats change without notice.
1. Charm testing uses unstable code that can break several times a day, preventing gathering and publication of data.
1. Charm testing leaves machines behind.
1. Charm testing can exceed resource limits in a cloud.
1. Charm testing doesn’t support multiple series.
1. Charm reports doesn’t show me a simple table of clouds a charm runs on.
	1. Most charms don’t have tests-- can we have a simple test to get every charm listed?
	1. I don’t know the version of the charm.
	1. I don’t know the last version that passed all tests.
1. Charm details reports don’t show me the individual tests.
	1. I don’t know the series.
	1. I don’t know the version that last passed the individual test.

### Work Items

1. Create a new jenkin that uses the last known good version of substrate dispatcher (lp:charmtester).
1. Staging charmworld or something will trigger a test of a branch and revision.
	1. Provide charmers with a script to test the MP/pull requests.
	1. Provide a way to poll Lp an Gh to automatically run the tests for the MP/PR.
	1. Provide a way to test tip of each promulgated charm.
1. Reporting needs to pick the data from the new test runner/jenkin.
1. Overview should list every charm tested.
	1. Does the charm have tests?
	1. A link to the specific charm results.
	1. Which clouds were tested and did the suite pass?
	1. What version was tested?
	1. What is the last known-good version to pass the tests for a substrate.
	1. What version passed all substrates.
1. For any charm, I need to see specific charm results.
	1. Which substrates were tested?
	1. The individual tests run in substrate, show name of the test and pass/fail.
	1. Need a link to see the fail log located somewhere.
	1. What was the last version of the charm to pass the test.
1. Update substrate dispatcher or switch to bundle tester to to gather richer data.
	1. Ensure `destroy-environment`.
	1. Capture and store JSDON data instead logs.
1. We will get use cases for the charm test reports that will verify the report meets expectations.
1. Tests could state their needed resources and the test runner can look to see if they are available. The tests can be deferred until resources are available.

## Charm testing with juju Core

1. We test with stable juju and charm.
1. We could test with unstable.
	1. Only test the popular charms for each revision.
	1. Or only test charm with tests.
	1. Or test bundles which has valid combinations.
1. Test all the charms occasionally.
1. Historically when charms break with new juju, it is the charm’s fault.

## Charm MP/Pull Gate on Charm Testing

Charm merges could be gated on a successful test run against the supported clouds.

- Allow charmers to manually request a test for a branch and revision.
- Maybe extend the script to poll for pull requests/merge proposals.
- Charm testing doesn’t support series testing yet.

### Testing

1. Test MP or pull request.
1. Test merge and commit on pass.
1. Charm testing runs and is actually testing that juju or ubuntu still works for the charm.

## CI Charm and Bundle Testing

Testing popular bundles with Juju unstable to ensure the charms and bundles continue to work.

1. Notify the charm maintainer or the juju developers when a break will happen.
1. Can testing be automated to grow newly popular charms and bundles?
1. There are resource limits per cloud.

### Notes

- Charm testing could be simplified to proof and unit tests.
- Bundle tests would test relations.
- Current tests don’t exercise failures or show error recovery.
- Ben suggests that amulet tests in charms are could be moved to bundles.
- Charms are like libraries, bundles are like applications.
	- Bundles are known topologies that we can support an recommend.
	- Charm tests could pass, but break other apps;  the bundle level where we want to test.
- Workloads are more like bundles, though some charms might be not need to in a relation, so a bundle of one.
- Config testing is valuable at the charm-level and bundle-level.
- Integration suites might work on a charm or a bundle.
	- Cloud-foundry tests only work with the bundle...running the suite for each charm means we construct the bundle multiple times and rerun tests.
- The charm author might right weak tests. Reviewer need to see this and respond. Bundles represent how users will use the charm, and that is what needs testing to verify utility and robustness.
- Bundle tester has a test pyramif.
	- Proofing each charm.
	- Discovering unit testing in each charm.
	- Discovering integration tests and running them.
- Bundle testing has a known set of resources...which is needed when testing in a cloud.
- Bundle tests provide the requirements for any software’s own stress and function tests.
- Charm reports would use the rich JSON data.

### Work Items

1. Review BenS Bundle testing for integration into QA Jenkins workflow
	1. Get back to BenS with any questions.
1. Use cases to drive what reports need to show.
	1. What do the different stakeholders need to discover reading the reports?
	1. What actions will stake holders take when reading the reports?
1. Do bundle tests poll for changes to bundles or the charms they use?
	1. The alternate would be to test on demand.
	1. Gated merges of MP/PR mean there is little value in testing on push.

## CI Ecosystem Tests

We want to extend Juju devel testing to verify that crucial ecosystems tools operate with it. When there is an error, the Juju-QA team will investigate and inform 1 or both owners of the issue that needs resolution.

The juju under test will be used with the other project’s test suite. A failure indicates Juju probably broke something, but maybe the other project was using juju in an unsupported way.

Juju CI will provide a simple functional test to demonstrate an example case works.

We want a prioritised list of tests to deliver.

1. Juju GUI
1. Juju Quickstart
1. Azure juju GUI dashboard
1. jass.io
1. Juju Deployer
1. mojo
1. amulet
1. charm tools
1. charm helpers
1. charmworld

### Work Items

1. Quickstart
	1. Quickstart relies on CLI and API, and config files. It waits for the GUI to come up in the env then deploy bundles.
	1. Quickstart opens a browser to show.
	1. Testing
		1. Install the proposed juju.
		1. Run juju-quickstart bundle to a bootstrapped env.
			1. Tries to colocate the bootstrap node and GUI when not local provider and the series and charm have the same series.
			1. Otherwise GUI is in a different container.
			1. `juju status` will list the charms from the bundle.
		1. Rerun juju-quickstart bundle.
			1. Verify the same env is running with eh same services.
	1. GUI team need to write
		1. Functional tests.
		1. Allow the tests to be run on lxc.
1. Juju GUI charm
	1. “make test” will deploy the charm about 8 times.
		1. GUI is deployed on bootstrap node to make the test faster.
		1. If the provider is local gui should be in a different container.
	1. The charms has tests that are run by juju test.
		1. The functional tests run the default juju.
		1. We can use the juju under test with the charm.
	1. An env variable is used select the series for the charm.
	1. Test with a bundle implicitly tests deployer.

## CI Cloud and Provider Testing

Juju CI tests deployments and upgrades from stable to release candidate. We might want additional tests.

1. Canonistack tests are disabled.
	1. Swift fails; IS suspect misconfiguration or bad name (rt 69317).
	1. Canonistack has bad days where no one can deploy.
1. Restricted and closed networks?
	1. CI has a restricted network test that shows the documented sites and ports are correct, but it doesn’t verify tools retrieval.
	1. A closed network test would have proxies providing every documented requirement of Juju.
1. Constraints?
1. Placement?
1. `add-machine`, `add-unit`?
1. Health checks for by series?

### Work Items

1. Placement tests are required for AWS and OpenStack.
1. `add-machine` and `add-unit` can be functional tests.
1. Need nova console log when we cannot ssh in.
1. Constraints are mostly
	1. Unique
		1. Azure availability sets (together relationship)
		1. AWS/OpenStack availability zones (apart relationship)
		1. Security groups
		1. MaaS networks

## CI Compatibility Function Testing

Juju CI has functional tests that exercise a function works across multiple versions of juju and when juju is working with multiple versions of itself.

1. Unstable to stable command line compatibility.
	1.  Verify deprecation, not obsolescence.
	1. Verify scripted arguments do not break after an upgrade.
1. 100% major.minor compatibility. Stable micro releases work with every combination?
	1. The means keeping a pool of stable packages for CI.
	1. Encourages creating new minor stables instead of adding test combinations; but SRU discourages minor releases.
	1. CI is **blocked** because Juju doesn’t allow anyone to specify the juju version to bootstrap the env with, nor can agent-metadata-url be set more than once to control the version found.

### Work Items

1. Juju bootstraps with the same version as the client.
1. Then juju upgrades/downgrades the other agents to the current version.
1. Ubuntu wants 100% compatibility between the client in trusty and all the servers that trusty has ever had.
	1. If trusty had juju 1.18.0, 1.18.1, 1.20.0, we need to show that clients work with all the servers.
1. We could parse the help and when an option disappears, we report bugs when options disappear. We need to see that commands and options are deprecated.
	1. We want to remove the deprecated features from the help to keep docs clean, but that makes deprecations look like obsolescence
1. Client to server is command line to API server.
	1. Standup each server, the for each client check that they talk.
	1. We don’t need to repeat historic combinations.
		Test the new client with the old servers.
		1. Test the old clients with the new servers.
1. The tests could be status, upgrade, and destroy, but if we had a API compatability check, we could quickly say the client and server are happy together
1. Maybe split the juju package to have a juju-server and juju-client package. Trusty gets the new juju client package. The servers are in the clouds.

## CI Feature Function Testing

Juju Command testing

1. Backup and restore (in progress).
1. HA
1. Charm hooks, relations, and expose, and upgrade-charm.
	1. Is the env setup for the hook.
	1. Do relations exchange info.
	1. Do expose/unexpose update ports?
	1. `upgrade-charm` downloads a charm and calls the upgrade hook.
1. ssh, scp, and run.
1. We claim gets the same env as a charm...we can test that the charm and run have the same env.
1. set/get config and environment.
	1. Which options are not mutable?

### Work Items

1. For every new feature we want to prepare a test that exercises it.
	1. Developer are interested in writing the tests with QA.
	1. Some tests may need to be run in several environments.
	1. Revise the docs about writing test and send them to developers.
1. Add coverage for historic features.
	1. `add-machine` / `add-unit`
	1. set/unset/get of config and env
	1. ssh, scp, and run
	1. charm hooks, relations, and expose, unexpose, and upgrade-charm
	1. init
	1. `get-constraints`, `generate-config`

## CI LTS (and other series and archs) Coverage

What is the right level of testing? Duplicate testing for each supported series may not be necessary. Unnecessary tests take time and limited cloud resources.

1. Can we test each series as an isolated case from clouds and providers?
1. Must we duplicate every cloud-provider test to ensure juju on each series in each cloud works.
1. Local provider seems to need a test for each series and juju.
1. Unit tests pass on amd64.
	1. PPC64el is close to passing.
	1. i386 and arm64 are not making progress.
1. Switch to golang 1.2.

### Work Items

1. The default test series will be trusty; precise is an exceptional case.
1. Golang will be 1.2.
	1. Golang 1.2 must be backported to precise and maybe saucy.
	1. If not, juju will have to abandon precise or only be 1.1.2 compatable.
1. Build juju on the real archs or cross compile to create tools.
	1. Build juju on trusty amd64.
	1. Build juju on precise amd64.
	1. Build juju on trusty i386.
	1. ppc64+trusty  will make gccgo-based juju.
	1. Need a machine to do arm64+trusty to make gccgo-based juju.
	1. Maybe CentOS.
	1. Maybe Win8 (agent for active server charm).
1. Remove the 386 unit tests, replace it wil a 386 client test.
1. Add tests for precise (where-as we had has special tests for trusty).
	1. Test a precise upgrade and deploy in one cloud.
1. Test each series+arch combination for local provider to confirm packaging and dependencies.
	1. precise+amd64 local
	1. trusty+amd64 local
	1. utopic+amd64 local
	1. trusty+ppc64 local
	1. trusty+arm64 local
1. Test client-server different series and arch to ensure the client’s series/arch does not influence the selection of tools.
	1. Utopic amd64 client bootstraps a trusty ppc64.
	1. We already test win juju client to juju precise amd64.

## CI MaaS and vMaaS

Juju CI had MaaS access to for 3 days. The tests ran with success. How do we ensure juju always works with MaaS?

1. CI wants 5 nodes.
1. CI wants the provider to be available at a moment's notice to run tests for the new revisions, just like all cloud are always available.
1. CI probably does care if MaaS is in hardware or virtualised. No public clouds support vMaaS today.

### Work Items

1. Ask Alexis, Mark R, and Robbie for mass hardware or access to stable MaaS env.

## CI KVM

Juju CI has local-provider KVM tests, but they cannot be run. Engineers have run them on their own machines.

1. CI wants 3 containers.
1. CI needs root access on real hardware (hence developers run on their machines).
1. CI does care about hardware; no public clouds support KVM today?

### Work Items

1. We can use the one of the 3 PPC machines.
1. We need to setup a slave in the network.
	1. Ideally we can add a machine and deploy Jenkins slave to it.
	1. Or we standup a slave without juju.
	1. Or we change the scripts to copy the tests to the machine.

## Juju in OIL

We think there may be interesting combinations to test. We know from bug reports that Juju didn’t support Havana’s multiple networks.

1. We want to know if Juju fails with new versions of OpenStack parts.
1. We want to know if Juju fails with some combinations of OpenStack.

## Vagrant

1. Run the virtual box image in a cloud.
	1. We care that the hosts mapping of dirs works with the image so that the charms are readable.
1. Exercise the local deployment.
	1. Deploy of local must work.
1. Failures might be
	1. Redirector of GUI failed.
		1. Packages in the image needed updating.
	1. lxc failed.
		1. Configuration of `env.yaml` might need changing.
		1. Command line deprecated or obsolete.
	1. When juju packaging deps change, the images need updating.
1. May need to communicate with Ben Howard to change the image.
1. Can CI pull images from a staging area to bless them?
1. Can we place the next juju into the virtual env to verify next juju works.

## Bug Triage and Planning

We have about 15 months of high bugs. Our planning cycles are 6 months. Though we are capable of fixing 400 bugs in this time, we know that 300 of the bugs are reported after planning. We, stakeholders, and customers need to know which bugs we intend to fix and those that  will only be fixed by opportunity or assistance.

1. Do we lower the priority of the 150  bugs?
	1. Do we make them medium? Medium bugs are not more likely to be fixed then low bugs...opportunity doesn’t discriminate by importance. We could say medium bugs are the first bugs to be re-triaged when we plan.
	1. Do we make them low? Low bugs obviously mean we don’t intend to fix the issue soon. Is it harder to re-triage all low bugs?
1. Do we create more milestones to organize work and show our intent? Can we plan work to be expedited instead of deferred?
	1. Target every bug we intend to address to a cycle milestone.
	1. Retarget some to major.minor milestones as we plan work.
	1. Retarget each to major.minor.micro milestones when branches merge.
1. Triaging every bug. Juju-GUI, deployer, charm-tools and a few others often have untriaged bugs that are week old. Who is responsible for them? https://bugs.launchpad.net/juju-project/+bugs?field.status=New&orderby=targetname

### Work Items

1. Want milestones that represent now, next stable, and cycle.
1. Now is the next release for the 2 week cycle.
	1. Team target the bugs they want to fix the cycle.
	1. We can see it burn down
1. Next stable are all the bugs  we think define a stable release.
	1. This doesn’t burn down because most bugs are retargeted. Some bugs will remain as they are the final bugs fixed to stable.
	1. 3 stable releases per 6-month cycle.
	1. Do we want a next next?
1. The cycle is 3 or 5 months that are all the high bugs we want to fix.
	1. We define stable milestones by pulling from the horizon milestone.
	1. Can we ensure there is a maximum capacity for the milestone? If you add a bug, you must remove the bug.
1. Critical
	1. CI breaks. QA team will do first level of analysis.
	1. Regressions are critical, but we may be reclassified.
	1. Critical bugs need to be assigned.
1. Flaky tests are High bugs in the current milestone.
1. Alexis and stakeholders will drive some bugs to be added or moved forward.
1. We have 15 months of high bugs
	1. To harden we need know which high bugs need fixing.
	1. We want to retriage all the high bugs and make most of them medium?
		1. Review the medium bugs regularly to promote them to high for the upcoming cycle or demote them to low.
	1. We want 75 bugs to be high at any one time (1 page of high bugs).

## Documentation

We want documentation written for the release notes before the release. We need greater collaboration to:

1. Know which features are in a release.
1. Know how the features work from the developer notes.
1. Include the docs to the release notes.
1. Developers review the release notes for errors.
1. Adequately document features in advance of release where possible.

We also need to discuss how versioning of the docs is going to work moving forward, and how we will manage and maintain separate versions of the docs, e.g. 1.18, 1.20, dev (unstable).

## MRE/SRU Juju into trusty

We want the current Juju to always be in trusty. We don’t like the cloud-archive because the current juju isn’t really in Ubuntu.

- Ubuntu wants guaranteed compatibility.
	- CI needs to ensure all versions of juju in a series work together.
- Landscape has an exception to keep current in all supported series.
	- Landscape only puts the client in supported series.
	- The server is in the clouds.
	- The client is stable, it changes slowly compared to the server.
	- The client works with many versions of the server, but tends to be used with the matching server.
- James Page suggests that juju be packaged with different names to permit co-installs. juju-1.20.0.

## Juju package delivers all the goodness

1. apt-get install juju could provide juju-core, charm-tools, deployer.

## juju-qa projects

1. Juju is moving to GitHub, Jerff and other canonical machines can only talk to Launchpad.
1. The ci-cd-scripts2 must be on Launchpad.
1. We must split the test branch from the juju project.
1. We may want to split the release scripts from test scripts.

# Juju Solutions

## Great Charm Audit of 2014

We've been doing an audit over the last couple of months -- and will continue.   We've scaled up the Charmers team from 2 people 5 months ago, to 7 or 8 by Vegas, so we are adding a lot more firepower on this front -- but that's all still new.   I expect to see significant increase on our charming capacity for the next cycle. 

## Pivotal Cloud Foundry Charms

Discussion points:

1. The pivot from packages to artifacts and why.
	1. Tarball of binaries for a given release.
	1. +1 on proceeding for orchestrating artifacts post Bosh build.
1. Altoros, internal staffing, schedule.
1. CF Service Brokers.
1. Brief look at current status, juju canvas.
1. What is demo-able by ODS?

## IBM Workloads

## ARM Workloads

## CABS

## Amtulet

- We want to know which charms are following an interface exchanged.
- When an interface is exchanged this is the information is passed.
- Then replay that.
- This boils down to we need an interface specification.  
- Mock up interface relations.
- Or figure out what the status is of the health check links.
- An opportunity to call the hook in integration suites.
- Could adopt some simplified version of Juju DB.
- They are talking about schema for next cycle.
- That probably isn’t the right answer.
- Someone would need to take over maintainership from Kapil.
- You need detailed knowledge of how Juju works.
- We want to know which charms are following an interface exchanged.
- When an interface is exchanged this is the information is passed.
- Then replay that.
- Build a quorum of what an interface looks like.
- This is the relation sentry in amulet.
- The problem with the relation sentry is the name is based on the
- Hacking around a problem that can be solved with tools in core.
- If core is not going to fix this, we need to hack round it.
- Bundle testing or Unit testing.  
- Is this portion of a deployment reusable to other?
- Depends on where we are going.
- 100% Certain bundle testing is the way of the future.
- Take some time writing a test and see how it would look.
- What is really needed?
- Do a single bundle test and see what that looks like it.
- Looking at this with a fresh set of eyes, may show us new aspects
- Once we go through the review of CI and see if we can.

## Charm Tools

## CharmWorld Lib

## Charm Helpers

- Folks interested: Chuck, Marco, Ben, Cory
- Break out contrib into charm helper contirib.
- Define a way to deliver.
- Where to I get it?
- How do I use it?
- What libraries are available?
- Actions
	- Delivery via the install hook.
	- Document
	- Move as much as possible out of contrib to core.
- Thursday  May 1
- Use doctest to ensure the documents are right.
	- doctest does not scale up very well.
- Unit test docs before promotion to core
- Move the things from outside of contrib and core into core
- Use Wheel packaging it is a blob format (make dist).
- Actually use and adhere to symantic versioning. 
	- This may include changes to  charm helpers sync to get the right version.  Fuzzy logic to find different versions.
- Chuck = Investigate the Altoris charm template for charm helpers

## Java Bundle

## HDP 2.0 Bundle 

- Create 12 charms for GA release of Hadoop Apache that Hortonworks supports.
	- http://hortonworks.com/hdp/
- Need to get communication from IBM on the porting of 12 components over to Power.
- Need to identify which HDP version is going to be the released version. 
- 3.0 will most likely be the next GA release.
- Need to support multi-language in the GUI.
- Next milestone:
	- Hadoop Summit demo.

## Big Data Roadmap

- Optimizations
	- File system via Juju through storage feature.
	-Image based Hadoop specific images.
- Conferences
	- Hadoop Summit (June)
	- Strata NY
- Demos
	- See how we can hook the Hadoop bundle into a charm framework bundle (e.g. Rails).
	- See how we can plug in multiple data sources.
		- Cancer, etc.
- Feature requests
	- Ensure that for services that need different fault domains/availability sets.
		- This may be resolved with tagging in MaaS.
			- Tag fault domain 1 and fault domain 2.
				- This is exposed to juju via the GUI.
	- Have the GUI/Landscape show which machines are in a given zone.
- Idea/need
	- We need to provide a means for Hadoop users to be able to put in their map-reduce java classes without having access to the admin portion of juju where hadoop is deployed.
		- The idea is to create a shim/relatoin/sub that provides a user level access to users to be able to add in their map-reduce  jobs.

## AmpLab Bundle

## Juju Actions in Bundles

## Charms in Git

## Charms Series Saga

## Fat Bundles and Caching Charms on Bootstrap Node

## Fat Charms in Closed Environments

- Detect Ports calling to the outside network.

## UA Charm Support Story

- Support bundles not charms.
- CTS validates the bundle relations and config.
- Has to have test.
- Need to have bundles in the charms store mark that it is UA supportable.

## How to engage Joyent & Altoros on provider support

## Unstable Doc Branches & Markdown

## Gating Charm merge proposals on charm testing passing

- Many useful relations.
- I expect this is very charm specific -- please feel free to list relations that we need.

## juju.ubuntu.ccom doc versioning

- Marco, Jorge, Curtis, Matthew
- Branches will be versions in Git.
	- 1.18
		- en
		- fr
	- 1.20
		- en
		- fr
- Branches will be versions in Git.
- How to generate  docs for live publishing.
- Juju QA team will build the markdown to HTML conversion.
	- In this conversation the Juju QA team will also incorporate the languages and drop down for versioning.
- Jorge to speak to the translations team on the best way forward.
- When committing to docs master the reviewer should also commit to unstable docs.
- Keep assets in a separate directory outside the versions and languages so we only have to updates one place for assets.

- Move author docs to a separate repository, but keep them in the nav for the live juju.ubuntu.com site.
	- The reason is that authoring docs should always be the current independent of the release.  Charm authoring should work the same across all releases. Thus, we should always show the latest.
		- The main idea is to de-couple the charm author docs for the user docs as we always want to show the latest charm author docs (as charm authoring should always work the same across releases).  This helps the scenario if we need to update the charm author docs we will need to update all the branches.
	- We will need to update the juju contributor docs once we move the charm author section.

## Juju and OpenStack

- Juju in keystone - Juju as a multi-tenant component registered in keystone.
- Juju in horizon - Juju gui and ui in horizon.
- Juju in heat - Juju / Deployer/bundle style exposed as dsl in heat.

# Juju GUI 

## Juju in OpenStack Horizon - Juju GUI in horizon

### Issues to resolve

- Embedding UI path? An OpenStack project or into an existing one.
- Embedding UI as far as framing/styling.
- Required timeframe, map out paths of resistance to make OpenStack release.
- The guiserver (python/tornado) running in that stack.
	- No bundles without deployer access.
		- Build deployer into core?
		- Build a full JS deployer?
	- No local charms file content.

## Juju in Azure - Juju GUI in Azure

### Issues to resolve

- Embedding UI path? Hosted externally and referenced in? Need to meet specific Azure tooling requirements?
- Embedding UI as far as framing/styling with existing Azure UX.
- Additional required functionality.
	- List environments.
- Required timeframe, map out paths of resistance to make deliverables.
- The guiserver (python/tornado) running in that stack.
	- No bundles without deployer access.
		- Build deployer into core?
		- Build a full JS deployer?
	- No local charms file content.

## Juju UI networks support

- Which types of networking supported and will be supported in core this cycle? Others planned to make sure design scales/works.
- What does design have for UX of this so far?
- Provider differences, sandbox, etc
- Make sure api exposure is complete enough in core to aid all UI team needs put forth by design.
	- Get anything not onto someone’s schedule.

## Juju UI Machine view 1.5

Most of this is a sync with design and check on what we put into 1.0 vs the final desired product.

- Deployed services inspector.
- Better search integration.
- Pre deployment config and visualization of bundles.
- Better local charms integration.
- Improved interactions (full drag/drop with the walkthrough/guide material).

## Juju UI Design Global Actions

We’ve got a series of tasks on the list that require is to find a way to represent things across the entire environment. We need to sit down with design and look at a common pattern to use for these ‘global’ environment-wide tools, many of which mirror tasks at the service, machine, and unit level.

### Items to discuss

- Design a home for global environment information.
- HA status/make HA.
- SSH Key management.
- Environment level debug-log.
- Environment level juju-run.

## In the trenches - customer feedback for GUI

The GUI team would like to meet with ecosystems and others selling/deploying the GUI in the field and get feedback on things we can and should look at doing to make the GUI a better tool and product. The goal is to help prioritize and give us ideas of paper cuts we should schedule to fix during maintenance time in the next cycle. 

## Juju UI Product Priorities

There’s a backlog of features to add to the GUI. We need a product team opinion on which to prioritize as we work around bigger tasks like Azure embedding. We won’t be able to get all done this cycle so we’d like feedback on those most useful to selling/marketing Juju.

- Debug log
- HA representation controls
- Network support
- Juju Run
- Multiple Users
- Fat bundles
- juju-quickstart on OS X
- juju-quickstart MaaS support
- SSH Key management UI

## Core Process Improvements

### Documentation

- Ian - use launchpad to track what bugs are where and which are fixed.
- Nate - an in-repo file is easier to keep track of, easier to verify during code reviews.

### Standups

- Leads meet once a week.
- Standups are squad standups.
- William 1 on 1s with leads.
- Team Leads email about team status.

### Vetting Ideas on Juju-dev

- Send user feature description to juju-dev before working on features.

### 2-Week Planning Cycle

- Dev release every 2 weeks.

### Contributing to CI tests

- We should do that.

### Move core to GitHub?

Needs to be scheduled and prioritized.  Non-zero work to get it working (build bot, process, etc).
 
- Code migration
- Code review
- Landing process
- Release process
- CI
- Documentation
- Private projects (ask Mark Ramm)

### Work Items

1. Code migration
	1. Do it all in one big migration.
	1. Namespace will be juju/core.
	1. Factor out others later.
	1. Disable GitHub bugtracker.
1. Code review
	1. Aim to use native GitHub code review.
	1. Find out about diffs being able to be expanded (ok, done).
	1. Rebase before issuing pull request to allow single revision to be cherry picked (investigate to be sure).
1. Branch setup
	1. Single trunk branch protected by bot.
1. Landing process
	1. Check out Rick’s lander branch (juju Jenkins GitHub lander).
	1. Run GitHub Jenkins lander on Jenkins CI instance.
1. Documentation
	1. Document entire process.
1. CI
	1. Polling for new revisions.
	1. Building release tarball


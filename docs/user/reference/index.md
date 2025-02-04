(reference)=
# Reference

<!--
Welcome to Juju Reference docs -- our cast of characters (tools, concepts, entities, and processes) for the Juju story!

When you install a Juju {ref}`client <client>`, for example the  {ref}`juju-cli`, and give Juju access to your {ref}`cloud <cloud-substrate>` (Kubernetes or otherwise), your Juju client {ref}`bootstraps <bootstrapping>` a  {ref}`controller <controller>` into the cloud. 

From that point onward you are officially a Juju  {ref}`user <user>` with a {ref}`superuser` access level <5348md>` and therefore able to use Juju and {ref}`charms <charm>` or {ref}`bundles <bundle>` from our large collection on {ref}`charmhub` to manage {ref}`applications <application>` on that cloud.

In fact, you can also go ahead and add another cloud definition to your controller, for any cloud in our long {ref}`list of supported clouds <list-of-supported-clouds>`.      

On any of the clouds, you can use the controller to set up a {ref}`model <model>`, and then use Juju for all your application management needs -- from application {ref}`deployment <deploying>` to {ref}`configuration <configuration>` to {ref}`constraints <constraint>` to {ref}`scaling <scaling>` to {ref}`high-availability <high-availability>`  to {ref}`integration <relation-integration>` (within and between models and their clouds!) to {ref}`actions <action>` to {ref}`secrets <secret>` to {ref}`upgrading <upgrading-things>` to {ref}`teardown <removing-things>`.     

You don't have to worry about the infrastructure -- the Juju controller {ref}`agent <agent>` takes care of all of that automatically for you. But, if you care, Juju also lets you manually control {ref}`availability zones <zone>`, {ref}`machines <machine>`, {ref}`subnets <subnet>`, {ref}`spaces <space>`, {ref}`secret backends <secret-backend>`, {ref}`storage <storage>`.                        
-->



```{toctree}
:titlesonly:
:glob:

action
agent
application
bundle
charm/index
cloud/index
configuration/index
constraint
containeragent
controller
credential
high-availability
hook
hook-commands/index
juju/index
juju-cli/index
jujuc
jujud
juju-dashboard
juju-web-cli
log
machine
model
offer
pebble
placement-directive
plugin/index
relation
removing-things
resource-charm
resource-compute/index
rockcraft
scaling
script
secret
space
ssh-key
status
storage
subnet
telemetry
unit
upgrading-things
user
worker
zone

```


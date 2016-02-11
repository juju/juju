// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package helptopics

const Glossary = `
Bootstrap
  To boostrap an model means initializing it so that Services may be
  deployed on it.

Charm
  A Charm provides the definition of the service, including its metadata,
  dependencies to other services, packages necessary, as well as the logic for
  management of the application. It is the layer that integrates an external
  application component like Postgres or WordPress into Juju. A Juju Service may
  generally be seen as the composition of its Juju Charm and the upstream
  application (traditionally made available through its package).

Charm URL
  A Charm URL is a resource locator for a charm, with the following format and
  restrictions:

    <schema>:[~<user>/]<collection>/<name>[-<revision>]

  schema must be either "cs", for a charm from the Juju charm store, or "local",
  for a charm from a local repository.

  user is only valid in charm store URLs, and allows you to source charms from
  individual users (rather than from the main charm store); it must be a valid
  Launchpad user name.

  collection denotes a charm's purpose and status, and is derived from the
  Ubuntu series targeted by its contained charms: examples include "precise",
  "quantal", "oneiric-universe".

  name is just the name of the charm; it must start and end with lowercase
  (ascii) letters, and can otherwise contain any combination of lowercase
  letters, digits, and "-"s.

  revision, if specified, points to a specific revision of the charm pointed to
  by the rest of the URL. It must be a non-negative integer.

Controller
  A Juju Controller, also sometimes called the controller model,
  describes the model that runs and manages the Juju API servers and the
  underlying database.

  The controller model is what is created when the bootstrap command is
  called. This controller model is a normal Juju model that just
  happens to have machines that manage Juju.

Endpoint
  The combination of a service name and a relation name.

Model
  An Model is a configured location where Services can be deployed onto.
  An Model typically has a name, which can usually be omitted when there's
  a single Model configured, or when a default is explicitly defined.
  Depending on the type of Model, it may have to be bootstrapped before
  interactions with it may take place (e.g. EC2). The local environment
  configuration is defined in an environments.yaml file inside $JUJU_DATA
  if said variable is not defined $XDG_DATA_HOME/juju will be used and if
  $XDG_DATA_HOME is not defined either it will default to 
  ~/.local/share/juju/ .

Machine Agent
  Software which runs inside each machine that is part of a Model, and is
  able to handle the needs of deploying and managing Service Units in this
  machine.

Placement Directive
  A provider-specific string that directs the provisioner on how to allocate a
  machine instance.

Provisioning Agent
  Software responsible for automatically allocating and terminating machines in
  a Model, as necessary for the requested configuration.

Relation
  Relations are the way in which Juju enables Services to communicate to each
  other, and the way in which the topology of Services is assembled. The Charm
  defines which Relations a given Service may establish, and what kind of
  interface these Relations require.

  In many cases, the establishment of a Relation will result into an actual TCP
  connection being created between the Service Units, but that's not necessarily
  the case. Relations may also be established to inform Services of
  configuration parameters, to request monitoring information, or any other
  details which the Charm author has chosen to make available.

Repository
  A location where multiple charms are stored. Repositories may be as simple as
  a directory structure on a local disk, or as complex as a rich smart server
  supporting remote searching and so on.

Service
  Juju operates in terms of services. A service is any application (or set of
  applications) that is integrated into the framework as an individual component
  which should generally be joined with other components to perform a more
  complex goal.

  As an example, WordPress could be deployed as a service and, to perform its
  tasks properly, might communicate with a database service and a load balancer
  service.

Service Configuration
  There are many different settings in a Juju deployment, but the term Service
  Configuration refers to the settings which a user can define to customize the
  behavior of a Service.

  The behavior of a Service when its Service Configuration changes is entirely
  defined by its Charm.

Service Unit
  A running instance of a given Juju Service. Simple Services may be deployed
  with a single Service Unit, but it is possible for an individual Service to
  have multiple Service Units running in independent machines. All Service Units
  for a given Service will share the same Charm, the same relations, and the
  same user-provided configuration.

  For instance, one may deploy a single MongoDB Service, and specify that it
  should run 3 Units, so that the replica set is resilient to failures.
  Internally, even though the replica set shares the same user-provided
  configuration, each Unit may be performing different roles within the replica
  set, as defined by the Charm.

Service Unit Agent
  Software which manages all the lifecycle of a single Service Unit.

Subnet
  A broadcast address range identified by a CIDR like 10.1.2.0/24, or 2001:db8::/32.

Space
  A collection of subnets which must be routable between each other without
  firewalls. A subnet can be in one and only one space. Connections between
  spaces instead are assumed to go through firewalls.

  Spaces and their subnets can span multiples zones, if supported by the
  cloud provider.

Zone
  Zones, also known as Availability Zones, are running on own physically
  distinct, independent infrastructure. They are engineered to be highly reliable.
  Common points of failures like generators and cooling equipment are not shared
  across Zones. Additionally, they are physically separate, such that even extremely
  uncommon disasters would only affect a single Zone.
`

> See also: [add-credential](#add-credential), [autoload-credentials](#autoload-credentials), [add-model](#add-model), [controller-config](#controller-config), [model-config](#model-config), [set-constraints](#set-constraints), [show-cloud](#show-cloud)

## Summary
Initializes a cloud environment.

## Usage
```juju bootstrap [options] [<cloud name>[/region] [<controller name>]]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--add-model` |  | Name of an initial model to create on the new controller |
| `--agent-version` |  | Version of agent binaries to use for Juju agents |
| `--auto-upgrade` | false | After bootstrap, upgrade to the latest patch release |
| `--bootstrap-base` |  | Specify the base of the bootstrap machine |
| `--bootstrap-constraints` | [] | Specify bootstrap machine constraints |
| `--bootstrap-image` |  | Specify the image of the bootstrap machine (requires --bootstrap-constraints specifying architecture) |
| `--bootstrap-series` |  | Specify the series of the bootstrap machine (deprecated use bootstrap-base) |
| `--build-agent` | false | Build local version of agent binary before bootstrapping |
| `--clouds` | false | Print the available clouds which can be used to bootstrap a Juju environment |
| `--config` |  | Specify a controller configuration file, or one or more configuration options. Model config keys only affect the controller model.     (--config config.yaml [--config key=value ...]) |
| `--constraints` | [] | Set model constraints |
| `--controller-charm-channel` | 3.6/stable | The Charmhub channel to download the controller charm from (if not using a local charm) |
| `--controller-charm-path` |  | Path to a locally built controller charm |
| `--credential` |  | Credentials to use when bootstrapping |
| `--db-snap` |  | Path to a locally built .snap to use as the internal juju-db service. |
| `--db-snap-asserts` |  | Path to a local .assert file. Requires --db-snap |
| `--force` | false | Allow the bypassing of checks such as supported series |
| `--keep-broken` | false | Do not destroy the provisioned controller instance if bootstrap fails |
| `--metadata-source` |  | Local path to use as agent and/or image metadata source |
| `--model-default` |  | Specify a configuration file, or one or more configuration     options to be set for all models, unless otherwise specified     (--model-default config.yaml [--model-default key=value ...]) |
| `--no-switch` | false | Do not switch to the newly created controller |
| `--regions` |  | Print the available regions for the specified cloud |
| `--storage-pool` |  | Specify options for an initial storage pool     'name' and 'type' are required, plus any additional attributes     (--storage-pool pool-config.yaml [--storage-pool key=value ...]) |
| `--to` |  | Placement directive indicating an instance to bootstrap |

## Examples

    juju bootstrap
    juju bootstrap --clouds
    juju bootstrap --regions aws
    juju bootstrap aws
    juju bootstrap aws/us-east-1
    juju bootstrap google joe-us-east1
    juju bootstrap --config=~/config-rs.yaml google joe-syd
    juju bootstrap --agent-version=2.2.4 aws joe-us-east-1
    juju bootstrap --config bootstrap-timeout=1200 azure joe-eastus
    juju bootstrap aws --storage-pool name=secret --storage-pool type=ebs --storage-pool encrypted=true
	juju bootstrap lxd --bootstrap-base=ubuntu@22.04

    # For a bootstrap on k8s, setting the service type of the Juju controller service to LoadBalancer
    juju bootstrap --config controller-service-type=loadbalancer

    # For a bootstrap on k8s, setting the service type of the Juju controller service to External
    juju bootstrap --config controller-service-type=external --config controller-external-name=controller.juju.is


## Details
Used without arguments, bootstrap will step you through the process of
initializing a Juju cloud environment. Initialization consists of creating
a 'controller' model and provisioning a machine to act as controller.

Controller names may only contain lowercase letters, digits and hyphens, and
may not start with a hyphen.
We recommend you call your controller ‘username-region’ e.g. ‘fred-us-east-1’.
See --clouds for a list of clouds and credentials.
See --regions &lt;cloud&gt; for a list of available regions for a given cloud.

Credentials are set beforehand and are distinct from any other
configuration (see `juju add-credential`).
The 'controller' model typically does not run workloads. It should remain
pristine to run and manage Juju's own infrastructure for the corresponding
cloud. Additional models should be created with `juju add-model` for workload purposes.
Note that a 'default' model is also created and becomes the current model
of the environment once the command completes. It can be discarded if
other models are created.

If '--bootstrap-constraints' is used, its values will also apply to any
future controllers provisioned for high availability (HA).

If '--constraints' is used, its values will be set as the default
constraints for all future workload machines in the model, exactly as if
the constraints were set with `juju set-model-constraints`.

It is possible to override constraints and the automatic machine selection
algorithm by assigning a "placement directive" via the '--to' option. This
dictates what machine to use for the controller. This would typically be
used with the MAAS provider ('--to &lt;host&gt;.maas').

You can change the default timeout and retry delays used during the
bootstrap by changing the following settings in your configuration
(all values represent number of seconds):

    # How long to wait for a connection to the controller
    bootstrap-timeout: 1200  # default: 20 minutes
    # How long to wait between connection attempts to a controller address.
    bootstrap-retry-delay: 5  # default: 5 seconds
    # How often to refresh controller addresses from the API server.
    bootstrap-addresses-delay: 10  # default: 10 seconds

It is possible to override the base e.g. ubuntu@22.04, Juju attempts 
to bootstrap on to, by supplying a base argument to '--bootstrap-base'.

An error is emitted if the determined base is not supported. Using the
'--force' option to override this check:

    juju bootstrap --bootstrap-base=ubuntu@22.04 --force

The '--bootstrap-series' flag can be still used, but is deprecated in favour
of '--bootstrap-base'.

Private clouds may need to specify their own custom image metadata and
tools/agent. Use '--metadata-source' whose value is a local directory.

By default, the Juju version of the agent binary that is downloaded and
installed on all models for the new controller will be the same as that
of the Juju client used to perform the bootstrap.
However, a user can specify a different agent version via '--agent-version'
option to bootstrap command. Juju will use this version for models' agents
as long as the client's version is from the same Juju release base.
In other words, a 2.2.1 client can bootstrap any 2.2.x agents but cannot
bootstrap any 2.0.x or 2.1.x agents.
The agent version can be specified a simple numeric version, e.g. 2.2.4.

For example, at the time when 2.3.0, 2.3.1 and 2.3.2 are released and your
agent stream is 'released' (default), then a 2.3.1 client can bootstrap:
   * 2.3.0 controller by running '... bootstrap --agent-version=2.3.0 ...';
   * 2.3.1 controller by running '... bootstrap ...';
   * 2.3.2 controller by running 'bootstrap --auto-upgrade'.
However, if this client has a copy of codebase, then a local copy of Juju
will be built and bootstrapped - 2.3.1.1.

Bootstrapping to a k8s cluster requires that the service set up to handle
requests to the controller be accessible outside the cluster. Typically this
means a service type of LoadBalancer is needed, and Juju does create such a
service if it knows it is supported by the cluster. This is performed by
interrogating the cluster for a well known managed deployment such as microk8s,
GKE or EKS.

When bootstrapping to a k8s cluster Juju does not recognise, there's no
guarantee a load balancer is available, so Juju defaults to a controller
service type of ClusterIP. This may not be suitable, so there are three bootstrap
options available to tell Juju how to set up the controller service. Part of
the solution may require a load balancer for the cluster to be set up manually
first, or perhaps an external k8s service via a FQDN will be used
(this is a cluster specific implementation decision which Juju needs to be
informed about so it can set things up correctly). The three relevant bootstrap
options are (see list of bootstrap config items below for a full explanation):

- controller-service-type
- controller-external-name
- controller-external-ips

Juju advertises those addresses to other controllers, so they must be resolveable from
other controllers for cross-model (cross-controller, actually) relations to work.

If a storage pool is specified using --storage-pool, this will be created
in the controller model.


Available keys for use with --config are:

Bootstrap configuration keys:

    admin-secret:
      type: string
      description: Sets the Juju administrator password
    bootstrap-addresses-delay:
      type: int
      description: Controls the amount of time in seconds in between refreshing the bootstrap
        machine addresses
    bootstrap-retry-delay:
      type: int
      description: Controls the amount of time in seconds between attempts to connect
        to a bootstrap machine address
    bootstrap-timeout:
      type: int
      description: Controls how long Juju will wait for a bootstrap to complete before
        considering it failed in seconds
    ca-cert:
      type: string
      description: Sets the bootstrapped controller's CA cert to use and issue certificates
        from, used in conjunction with ca-private-key
    ca-private-key:
      type: string
      description: Sets the bootstrapped controller's CA cert private key to sign certificates
        with, used in conjunction with ca-cert
    controller-external-ips:
      type: list
      description: Specifies a comma separated list of external IPs for a k8s controller
        of type external
    controller-external-name:
      type: string
      description: Sets the external name for a k8s controller of type external
    controller-service-type:
      type: string
      description: |-
        Controls the kubernetes service type for Juju controllers, see
        https://kubernetes.io/docs/reference/kubernetes-api/service-resources/service-v1/#ServiceSpec
        valid values are one of cluster, loadbalancer, external
    ssh-server-host-key:
      type: string
      description: Sets the bootstrapped controller's SSH server host key
    

Controller configuration keys:

    agent-logfile-max-backups:
      type: int
      description: The number of old agent log files to keep (compressed)
    agent-logfile-max-size:
      type: string
      description: The maximum size of the agent log file
    agent-ratelimit-max:
      type: int
      description: The maximum size of the token bucket used to ratelimit agent connections
    agent-ratelimit-rate:
      type: string
      description: The time taken to add a new token to the ratelimit bucket
    allow-model-access:
      type: bool
      description: "Determines if the controller allows users to \nconnect to models they
        have been authorized for even when \nthey don't have any access rights to the
        controller itself"
    api-port:
      type: int
      description: The port used for api connections
    api-port-open-delay:
      type: string
      description: "The duration that the controller will wait \nbetween when the controller
        has been deemed to be ready to open \nthe api-port and when the api-port is actually
        opened \n(only used when a controller-api-port value is set)."
    application-resource-download-limit:
      type: int
      description: The maximum number of concurrent resources downloads per application
    audit-log-capture-args:
      type: bool
      description: Determines if the audit log contains the arguments passed to API methods
    audit-log-exclude-methods:
      type: list
      description: The list of Facade.Method names that aren't interesting for audit logging
        purposes.
    audit-log-max-backups:
      type: int
      description: The number of old audit log files to keep (compressed)
    audit-log-max-size:
      type: string
      description: The maximum size for the current controller audit log file
    auditing-enabled:
      type: bool
      description: Determines if the controller records auditing information
    autocert-dns-name:
      type: string
      description: The DNS name of the controller
    autocert-url:
      type: string
      description: The URL used to obtain official TLS certificates when a client connects
        to the API
    caas-image-repo:
      type: string
      description: The docker repo to use for the jujud operator and mongo images
    caas-operator-image-path:
      type: string
      description: |-
        (deprecated) The url of the docker image used for the application operator.
        Use "caas-image-repo" instead.
    controller-api-port:
      type: int
      description: |-
        An optional port that may be set for controllers
        that have a very heavy load. If this port is set, this port is used by
        the controllers to talk to each other - used for the local API connection
        as well as the pubsub forwarders, and the raft workers. If this value is
        set, the api-port isn't opened until the controllers have started properly.
    controller-name:
      type: string
      description: The canonical name of the controller
    controller-resource-download-limit:
      type: int
      description: The maximum number of concurrent resources downloads across all the
        applications on the controller
    features:
      type: list
      description: A list of runtime changeable features to be updated
    identity-public-key:
      type: string
      description: The public key of the identity manager
    identity-url:
      type: string
      description: The url of the identity manager
    juju-db-snap-channel:
      type: string
      description: Sets channel for installing mongo snaps when bootstrapping on focal
        or later
    juju-ha-space:
      type: string
      description: The network space within which the MongoDB replica-set should communicate
    juju-mgmt-space:
      type: string
      description: The network space that agents should use to communicate with controllers
    jujud-controller-snap-source:
      type: string
      description: The source for the jujud-controller snap.
    login-token-refresh-url:
      type: string
      description: The url of the jwt well known endpoint
    max-agent-state-size:
      type: int
      description: The maximum size (in bytes) of internal state data that agents can
        store to the controller
    max-charm-state-size:
      type: int
      description: The maximum size (in bytes) of charm-specific state that units can
        store to the controller
    max-debug-log-duration:
      type: string
      description: The maximum duration that a debug-log session is allowed to run
    max-prune-txn-batch-size:
      type: int
      description: (deprecated) The maximum number of transactions evaluated in one go
        when pruning
    max-prune-txn-passes:
      type: int
      description: (deprecated) The maximum number of batches processed when pruning
    max-txn-log-size:
      type: string
      description: The maximum size the of capped txn log collection
    metering-url:
      type: string
      description: The url for metrics
    migration-agent-wait-time:
      type: string
      description: The maximum during model migrations that the migration worker will
        wait for agents to report on phases of the migration
    model-logfile-max-backups:
      type: int
      description: The number of old model log files to keep (compressed)
    model-logfile-max-size:
      type: string
      description: The maximum size of the log file written out by the controller on behalf
        of workers running for a model
    model-logs-size:
      type: string
      description: The size of the capped collections used to hold the logs for the models
    mongo-memory-profile:
      type: string
      description: Sets mongo memory profile
    prune-txn-query-count:
      type: int
      description: The number of transactions to read in a single query
    prune-txn-sleep-time:
      type: string
      description: The amount of time to sleep between processing each batch query
    public-dns-address:
      type: string
      description: Public DNS address (with port) of the controller.
    query-tracing-enabled:
      type: bool
      description: Enable query tracing for the dqlite driver
    query-tracing-threshold:
      type: string
      description: "The minimum duration of a query for it to be traced. The lower the
        \nthreshold, the more queries will be output. A value of 0 means all queries \nwill
        be output if tracing is enabled."
    set-numa-control-policy:
      type: bool
      description: Determines if the NUMA control policy is set
    ssh-max-concurrent-connections:
      type: int
      description: The maximum number of concurrent ssh connections to the controller
    ssh-server-port:
      type: int
      description: The port used for ssh connections to the controller
    state-port:
      type: int
      description: The port used for mongo connections
    
Model configuration keys (affecting the controller model):

    agent-metadata-url:
      type: string
      description: URL of private stream
    agent-stream:
      type: string
      description: Version of Juju to use for deploy/upgrades.
    apt-ftp-proxy:
      type: string
      description: The APT FTP proxy for the model
    apt-http-proxy:
      type: string
      description: The APT HTTP proxy for the model
    apt-https-proxy:
      type: string
      description: The APT HTTPS proxy for the model
    apt-mirror:
      type: string
      description: The APT mirror for the model
    apt-no-proxy:
      type: string
      description: List of domain addresses not to be proxied for APT (comma-separated)
    authorized-keys:
      type: string
      description: Any authorized SSH public keys for the model, as found in a ~/.ssh/authorized_keys
        file
    automatically-retry-hooks:
      type: bool
      description: Determines whether the uniter should automatically retry failed hooks
    backup-dir:
      type: string
      description: Directory used to store the backup working directory
    charmhub-url:
      type: string
      description: The url for CharmHub API calls
    cloudinit-userdata:
      type: string
      description: Cloud-init user-data (in yaml format) to be added to userdata for new
        machines created in this model
    container-image-metadata-defaults-disabled:
      type: bool
      description: Whether default simplestreams sources are used for image metadata with
        containers.
    container-image-metadata-url:
      type: string
      description: The URL at which the metadata used to locate container OS image ids
        is located
    container-image-stream:
      type: string
      description: The simplestreams stream used to identify which image ids to search
        when starting a container.
    container-inherit-properties:
      type: string
      description: List of properties to be copied from the host machine to new containers
        created in this model (comma-separated)
    container-networking-method:
      type: string
      description: Method of container networking setup - one of fan, provider, local
    default-base:
      type: string
      description: The default base image to use for deploying charms, will act like --base
        when deploying charms
    default-space:
      type: string
      description: The default network space used for application endpoints in this model
    development:
      type: bool
      description: Whether the model is in development mode
    disable-network-management:
      type: bool
      description: Whether the provider should control networks (on MAAS models, set to
        true for MAAS to control networks
    disable-telemetry:
      type: bool
      description: Disable telemetry reporting of model information
    egress-subnets:
      type: string
      description: Source address(es) for traffic originating from this model
    enable-os-refresh-update:
      type: bool
      description: Whether newly provisioned instances should run their respective OS's
        update capability.
    enable-os-upgrade:
      type: bool
      description: Whether newly provisioned instances should run their respective OS's
        upgrade capability.
    extra-info:
      type: string
      description: Arbitrary user specified string data that is stored against the model.
    fan-config:
      type: string
      description: Configuration for fan networking for this model
    firewall-mode:
      type: string
      description: |-
        The mode to use for network firewalling.
    
        'instance' requests the use of an individual firewall per instance.
    
        'global' uses a single firewall for all instances (access
        for a network port is enabled to one instance if any instance requires
        that port).
    
        'none' requests that no firewalling should be performed
        inside the model. It's useful for clouds without support for either
        global or per instance security groups.
    ftp-proxy:
      type: string
      description: The FTP proxy value to configure on instances, in the FTP_PROXY environment
        variable
    http-proxy:
      type: string
      description: The HTTP proxy value to configure on instances, in the HTTP_PROXY environment
        variable
    https-proxy:
      type: string
      description: The HTTPS proxy value to configure on instances, in the HTTPS_PROXY
        environment variable
    ignore-machine-addresses:
      type: bool
      description: Whether the machine worker should discover machine addresses on startup
    image-metadata-defaults-disabled:
      type: bool
      description: Whether default simplestreams sources are used for image metadata.
    image-metadata-url:
      type: string
      description: The URL at which the metadata used to locate OS image ids is located
    image-stream:
      type: string
      description: The simplestreams stream used to identify which image ids to search
        when starting an instance.
    juju-ftp-proxy:
      type: string
      description: The FTP proxy value to pass to charms in the JUJU_CHARM_FTP_PROXY environment
        variable
    juju-http-proxy:
      type: string
      description: The HTTP proxy value to pass to charms in the JUJU_CHARM_HTTP_PROXY
        environment variable
    juju-https-proxy:
      type: string
      description: The HTTPS proxy value to pass to charms in the JUJU_CHARM_HTTPS_PROXY
        environment variable
    juju-no-proxy:
      type: string
      description: List of domain addresses not to be proxied (comma-separated), may contain
        CIDRs. Passed to charms in the JUJU_CHARM_NO_PROXY environment variable
    logforward-enabled:
      type: bool
      description: Whether syslog forwarding is enabled.
    logging-config:
      type: string
      description: The configuration string to use when configuring Juju agent logging
        (see http://godoc.org/github.com/juju/loggo#ParseConfigurationString for details)
    logging-output:
      type: string
      description: 'The logging output destination: database and/or syslog. (default "")'
    lxd-snap-channel:
      type: string
      description: The channel to use when installing LXD from a snap (cosmic and later)
    max-action-results-age:
      type: string
      description: The maximum age for action entries before they are pruned, in human-readable
        time format
    max-action-results-size:
      type: string
      description: The maximum size for the action collection, in human-readable memory
        format
    max-status-history-age:
      type: string
      description: The maximum age for status history entries before they are pruned,
        in human-readable time format
    max-status-history-size:
      type: string
      description: The maximum size for the status history collection, in human-readable
        memory format
    mode:
      type: string
      description: |-
        Mode is a comma-separated list which sets the
        mode the model should run in. So far only one is implemented
        - If 'requires-prompts' is present, clients will ask for confirmation before removing
        potentially valuable resources.
        (default "")
    net-bond-reconfigure-delay:
      type: int
      description: The amount of time in seconds to sleep between ifdown and ifup when
        bridging
    no-proxy:
      type: string
      description: List of domain addresses not to be proxied (comma-separated)
    num-container-provision-workers:
      type: int
      description: The number of container provisioning workers to use per machine
    num-provision-workers:
      type: int
      description: The number of provisioning workers to use per model
    provisioner-harvest-mode:
      type: string
      description: What to do with unknown machines (default destroyed)
    proxy-ssh:
      type: bool
      description: Whether SSH commands should be proxied through the API server
    resource-tags:
      type: attrs
      description: resource tags
    saas-ingress-allow:
      type: string
      description: |-
        Application-offer ingress allowlist is a comma-separated list of
        CIDRs specifying what ingress can be applied to offers in this model.
    secret-backend:
      type: string
      description: The name of the secret store backend. (default "auto")
    snap-http-proxy:
      type: string
      description: The HTTP proxy value for installing snaps
    snap-https-proxy:
      type: string
      description: The HTTPS proxy value for installing snaps
    snap-store-assertions:
      type: string
      description: The assertions for the defined snap store proxy
    snap-store-proxy:
      type: string
      description: The snap store proxy for installing snaps
    snap-store-proxy-url:
      type: string
      description: The URL for the defined snap store proxy
    ssh-allow:
      type: string
      description: |-
        SSH allowlist is a comma-separated list of CIDRs from
        which machines in this model will accept connections to the SSH service.
        Currently only the aws & openstack providers support ssh-allow
    ssl-hostname-verification:
      type: bool
      description: Whether SSL hostname verification is enabled (default true)
    storage-default-block-source:
      type: string
      description: The default block storage source for the model
    storage-default-filesystem-source:
      type: string
      description: The default filesystem storage source for the model
    syslog-ca-cert:
      type: string
      description: The certificate of the CA that signed the syslog server certificate,
        in PEM format.
    syslog-client-cert:
      type: string
      description: The syslog client certificate in PEM format.
    syslog-client-key:
      type: string
      description: The syslog client key in PEM format.
    syslog-host:
      type: string
      description: The hostname:port of the syslog server.
    test-mode:
      type: bool
      description: |-
        Whether the model is intended for testing.
        If true, accessing the charm store does not affect statistical
        data of the store. (default false)
    transmit-vendor-metrics:
      type: bool
      description: Determines whether metrics declared by charms deployed into this model
        are sent for anonymized aggregate analytics
    update-status-hook-interval:
      type: string
      description: How often to run the charm update-status hook, in human-readable time
        format (default 5m, range 1-60m)
    





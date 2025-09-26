(juju-security)=
# Juju security

```{ibnote}
See also: {ref}`harden-your-deployment`
```

Juju is a distributed system where users interact with clients to reach controllers that will contact clouds and Charmhub to provision infrastructure and to deploy and operate workloads on that infrastructure through charms.

Whether inadvertently or maliciously, any of these assets and the data flows between them can be compromised.

This document describes example threats and the available controls in each case.

(security-assets)=
## Assets

This section lists all the main assets along with example threats and the types of controls Juju makes available.

Note: Juju groups these assets under various abstractions: {ref}`controllers <controller>`, {ref}`clouds <cloud>`, {ref}`models <model>`, {ref}`applications <application>`, {ref}`units <unit>`, etc. Juju's permission model also refers to these abstractions (see {ref}`user-access-levels`: a user can have read, write, or admin access to controllers, clouds, models, or application offers). Below we allude to them in the "Owned by" and "Used by" fields as needed.

(assets-agents)=
### Agents

Agents are Juju software that lives on every machine or unit managed by Juju and helps reconcile local state with the goal state as represented in the Juju database.

```{ibnote}
See more: {ref}`Agents <agent>`
```

In this document the term "agent" will denote only machine, container, and unit agents; for the controller agent see {ref}`the controller asset <assets-controller>`.

Owned by: Controller.

Used by: Models.

(assets-unit-agents)=
#### Unit agents

Unit agents are deployed next to each charm. They periodically check local state against the Juju controller and work to reconcile state by executing the deployed charm, which in turn reacts, typically by performing an operation on their workload.

Privileges: Unit agents typically have root access on the machine or container where they are deployed.

Controls: {ref}`controls-rootless-charms`

### Blob storage

Owned by: Controller.

Used by: Models.

Storage for large objects, e.g., charm resources, backups, etc.

Example threats:
- {ref}`threats-availability`: An attacker could launch a denial-of-service (DoS) attack on Juju's blob storage, rendering it unavailable for legitimate users and processes.
- {ref}`threats-confidentiality`: If an attacker gains unauthorized access to Juju's blob storage, they could potentially view and extract sensitive data stored in the blobs, such as configuration files or application data.
- {ref}`threats-integrity`: An attacker who gains access to Juju's blob storage could modify or corrupt stored data, such as altering configuration files and injecting malicious code into application data.

Controls: {ref}`controls-tls-encryption`

(assets-charms)=
### Charms

Charms are the operational code Juju deploys and manages.

Owned by: External entity.

Used by: Models.

Example threats:
- {ref}`threats-confidentiality`: Charms may need to use sensitive information such as database credentials, API keys, or encryption keys to function properly. If these secrets are not managed securely, they can be exposed to unauthorized users or entities.

Controls: {ref}`controls-secrets`, {ref}`controls-secret-backends-can-be-external`, {ref}`controls-charm-development-best-practices`

<!--TBA
(assets-charmhub)=
### Charmhub

Charmhub is the charm repository where the charms deployed by Juju are by default retrieved from.

Example threats:
- {ref}`threats-availability`:
- {ref}`threats-confidentiality`:
- {ref}`threats-integrity`:

Controls: {ref}`controls-`
-->

(assets-clients)=
### Clients

Overview: Juju {ref}`clients <client>` interact with the API server via client interfaces.

<!-- The `juju` CLI can be installed from a strictly confined snap. The strict confinement means the client cannot do malicious things. The snap means the client updates by itself.

When you install the `juju` CLI you also get the agent binaries necessary for deploying Juju agents during controller bootstrap, infrastructure provisioning, or application deployment. -->

Owned by: Client, controller.

Used by: Models.

Example threats:
- {ref}`threats-availability`: An attacker could target the availability of Juju clients by disrupting their operation, such as through a denial-of-service (DoS) attack on the JAAS service or by corrupting local client installations.
- {ref}`threats-confidentiality`: If communication between Juju clients and the Juju controller is not encrypted, an attacker could intercept the data, leading to the exposure of sensitive information, such as credentials, configuration details, or command history.
- {ref}`threats-integrity`: An attacker with access to the Juju client environment could modify configuration files or scripts, leading to unauthorized or malicious actions being executed by the Juju clients.

Controls: {ref}`controls-user-authentication`

(assets-cloud-credentials)=
### Cloud credentials

Overview: Cloud credentials allow Juju to interact with various cloud providers for deploying and managing resources.

Owned by: Client, controller. Alternatively, for clouds that support instance profiles or a managed identity, owned externally.

Used by: Models. If a charm must be deployed with `--trust`, it will gain access to cloud credentials too.

Example threats:
- {ref}`threats-availability`: If cloud credentials are lost, deleted, or become unavailable, Juju may be unable to interact with cloud resources, leading to disruptions in deployments or scaling operations.
- {ref}`threats-confidentiality`: If an attacker gains unauthorized access to Juju cloud credentials, they could potentially control cloud resources, create or destroy instances, and access sensitive data stored in the cloud environment.
- {ref}`threats-integrity`: An attacker could modify Juju cloud credentials, such as altering access keys or API tokens, which could either lock out legitimate users or allow unauthorized access to cloud resources.

Controls: {ref}`controls-filesystem-permissions`, {ref}`controls-no-cloud-credentials-in-juju`

(assets-cloud-providers)=
### Cloud providers

Overview: All the clouds supported by Juju: Amazon EC2, Google GCP, Oracle OCI, Microsoft Azure, Kubernetes, Openstack, etc.

```{ibnote}
See more: {ref}`list-of-supported-clouds`
```

Example threats:
- {ref}`threats-availability`: An attacker could intentionally disrupt cloud services by terminating critical instances, exhausting resources (e.g., through a denial-of-service attack), or misconfiguring network settings, leading to the unavailability of services managed by Juju.
- {ref}`threats-confidentiality`: If an attacker gains unauthorized access to a cloud provider account used by Juju, they could potentially view and extract sensitive data from the cloud environment, including virtual machines, storage, and network configurations.
- {ref}`threats-integrity`: An attacker with access to the cloud provider account could modify or delete resources, such as altering virtual machine configurations, modifying storage contents, or changing network settings.

Controls: {ref}`controls-filesystem-permissions`, {ref}`controls-no-plaintext-passwords-in-the-database`

(assets-cloud-images)=
### Cloud images

Overview: Cloud images are retrieved from cloud-images.ubuntu.com.

Owned by: External entity.

Used by: Models.

Example threats:
- {ref}`threats-availability`: An attacker could launch a Denial of Service (DoS) attack on cloud-images.canonical.com, rendering it unavailable for Juju to download necessary cloud images.
- {ref}`threats-integrity`: When Juju accesses cloud-images.canonical.com to download cloud images, an attacker could perform a Man-in-the-Middle (MitM) attack. In this scenario, the attacker intercepts the connection between Juju and the image repository, potentially injecting or modifying the images being downloaded.

Controls: {ref}`controls-tls-encryption`

<!--?TODO?
### Cloud resources

Cloud resources are any machines or containers provisioned implicitly or explicitly through Juju for the purpose of hosting a Juju controller or other applications.

By default, these resources run Ubuntu.

For the controller, this is always the latest Ubuntu LTS.
-->

(assets-container-registry)=
### Container registry

Owned by: External entity.

Used by: Models.

Example threats:
- {ref}`threats-availability`: An attacker could launch a denial-of-service (DoS) attack against the Juju container registry, making it unavailable to users who need to pull images for deployments.
- {ref}`threats-confidentiality`: If an attacker gains unauthorized access to the Juju container registry, they could download private container images that may contain sensitive information, such as proprietary application code, embedded secrets, or configuration details.
- {ref}`threats-integrity`: An attacker could potentially push malicious container images to the Juju container registry, replacing legitimate images or adding new ones. If these tampered images are deployed, they could compromise the security and integrity of the applications running in the Juju environment.

Controls: {ref}`controls-tls-encryption`

(assets-controller)=
### Controller

Overview: A Juju controller is the central management entity that handles state management, event processing, and coordination of operations across models and units. The controller provides a RESTful API for external interactions with the Juju system, including CLI operations and third-party integrations.

<!-- A Juju controller unit consists of a Juju unit agent, `juju-controller` charm code, a controller agent, and `juju-db` -- Juju's database and occupies a machine or containers in a pod.

A controller on a machine cloud can operate in high availability mode.

For machine controllers, Juju also provides tools to help with controller backups. This can help restore healthy state in the case of an attack affecting data integrity. -->

Owned by: Controller.

Used by: Controller.

Example threats:
- {ref}`threats-availability`: An attacker could flood the API server with requests, overwhelming its capacity and rendering it unavailable to legitimate users. Or, improper resource management might lead to exhaustion of system resources (e.g., CPU, memory), causing the API server to crash or become unresponsive.
- {ref}`threats-confidentiality`:  If user credentials are compromised, unauthorized individuals could gain access to sensitive data via API server.
- {ref}`threats-integrity`: An attacker who gains unauthorized access to the APIserver could alter critical data, such as configuration settings or deployment scripts. Malicious users could exploit vulnerabilities in the API to execute injection attacks, such as SQL injection or command injection, altering the behavior of the API server.

Controls:  {ref}`controls-vpn`, {ref}`controls-user-authentication`, {ref}`controls-time-limited-tokens`, {ref}`controls-filesystem-permissions`, {ref}`controls-rate-limiting`, {ref}`controls-regular-backups`

(assets-controller-ca)=
### Controller CA

Overview: Juju configures everything that talks to the API to trust certificates that have been signed by the CA that the controller manages. This establishes trust for both clients and agents.

Owned by: Controller.

Used by: Client, agents.

Example threats:
- {ref}`threats-availability`: An attacker could launch a denial-of-service (DoS) attack against the controller CA, making it unavailable to issue or renew certificates.
- {ref}`threats-confidentiality`: If an attacker gains unauthorized access to the private keys of the controller CA, they could use these keys to issue fraudulent certificates. This would allow them to impersonate legitimate Juju controllers or services, leading to a breach of confidentiality as they could decrypt secure communications or authenticate unauthorized systems.
- {ref}`threats-integrity`: An attacker who compromises the controller CA could issue fraudulent certificates to malicious entities, allowing them to impersonate legitimate services within the Juju environment.

Controls: {ref}`controls-database-authentication`

(assets-controller-charm)=
### Controller charm

Overview: https://charmhub.io/juju-controller : A charm responsible for some of the operational logic of a Juju controller.

Owned by: Controller.

Used by: Controller model.

Example threats:
- {ref}`threats-availability`: A misconfiguration in the Juju controller charm could lead to service disruptions, such as the controller becoming unresponsive or unable to manage deployed applications.
- {ref}`threats-confidentiality`: If an attacker gains unauthorized access to the Juju controller charm, they could potentially view sensitive configuration data, including credentials, API keys, and other secrets managed by the charm.
- {ref}`threats-integrity`:  An attacker who gains access to the Juju controller charm could modify the charm's code or configuration, potentially introducing vulnerabilities, backdoors, or malicious behavior.

Controls: {ref}`controls-user-authentication`

(assets-database)=
### Database

Overview: Juju stores all its state and operational data in a database powered by Dqlite or MongoDB. This includes model configurations, status of applications, and historical logs. This database can only be accessed by authorized entities (controllers, agents, or administrators), following proper authentication. All passwords saved in the database are hashed and salted. Juju is careful not to store sensitive information in logs.

Owned by: Controller.

Used by: Controller.

Example threats:
- {ref}`threats-availability`: An attacker could overwhelm the database with requests, rendering it unavailable to legitimate users and disrupting the operations of the Juju-managed environment.
- {ref}`threats-confidentiality`: If unauthorized users gain access to the Juju database, they can read sensitive data stored in it, such as configuration details, user credentials, or operational data.
- {ref}`threats-integrity`: If unauthorized users gain access to the Juju database, they can read sensitive data stored in it, such as configuration details, user credentials, or operational data.

Controls: {ref}`controls-high-availability`, {ref}`controls-database-authentication`, {ref}`controls-filesystem-permissions`, {ref}`controls-no-plaintext-passwords-in-the-database`, {ref}`controls-regular-backups`

(assets-image-registry)=
### Image registry

Managed by CPC, keeps the official record of what Ubuntu images exist in what regions of AWS, Azure, Google Cloud, Oracle, etc.

Owned by: External entity.

Used by: Models.

Example threats:
- {ref}`threats-availability`: An attacker could launch a Denial of Service (DoS) attack against the Juju image registry, overwhelming it with traffic or requests, rendering it unavailable.
- {ref}`threats-integrity`: An attacker could gain unauthorized access to the Juju image registry and inject malicious images or alter existing images.

Controls: {ref}`controls-filesystem-permissions`

(assets-logging-and-monitoring-systems)=
### Logging and monitoring systems

Overview: Any systems that capture operational data and alerts for Juju's activities.

Owned by: Controller.

Used by: Controller, models.

Example threats:

- {ref}`threats-confidentiality`: Juju's logging and monitoring system might inadvertently capture sensitive information, such as passwords, API keys, or personally identifiable information (PII), within log files. If these logs are not properly secured, unauthorized users could access them, leading to a confidentiality breach.

Controls: {ref}`controls-no-sensitive-information-in-logs`

<!-- TODO
(assets-pebble)=
### Pebble


Example threats:
- {ref}`threats-availability`:
- {ref}`threats-confidentiality`:
- {ref}`threats-integrity`:

Controls: {ref}`controls-`
-->

(assets-secrets)=
### Secrets

Overview: Juju's mechanism for storing and managing sensitive information such as credentials and API keys.

Owned by: Controller.

Used by: Models.

Example threats:
- {ref}`threats-availability`: An attacker could potentially launch a denial-of-service attack on the secrets management component, preventing legitimate users or charms from accessing the secrets they require.
- {ref}`threats-confidentiality`: If a malicious actor gains unauthorized access to the secrets stored within the Juju model, they could potentially extract sensitive information such as API keys, passwords, or encryption keys.
- {ref}`threats-integrity`: An attacker could modify the secrets stored in the Juju model.

Controls: {ref}`controls-secrets`, {ref}`controls-secret-backend-can-be-external`, {ref}`controls-user-authentication`

(assets-ssh-keys-and-agent-credentials)=
### SSH keys and agent credentials

Overview: SSH keys are used for secure communication and access to machines managed by Juju. Agent credentials (e.g., macaroons) are used by agents to communicate wth external controllers (in cross-model-relation scenarios).

Example threats:
- {ref}`threats-availability`: If SSH keys or credentials are lost or become unavailable, administrators may be unable to manage or access critical resources, leading to operational disruptions.
- {ref}`threats-confidentiality`: If an attacker gains access to Juju SSH keys or credentials, they can potentially access and control machines or services managed by Juju.
- {ref}`threats-integrity`: An attacker might alter SSH keys or credentials, replacing them with their own, which could allow them unauthorized access or disrupt the normal operations of Juju-managed resources.

Controls: {ref}`controls-user-authentication`, {ref}`controls-filesystem-permissions`

(assets-simplestreams)=
### Simplestreams

Overview: Hosts the Juju agent binaries.

Owned by: External entity.

Used by: Controller, models.

Example threats:
- {ref}`threats-integrity`: When Juju accesses streams.canonical.com to retrieve images, charm metadata, or other critical resources, an attacker could perform a Man-in-the-Middle (MitM) attack. In such a scenario, the attacker intercepts the communication between Juju and streams.canonical.com and injects or modifies the data being transmitted.

Controls: {ref}`controls-tls-encryption`

(assets-users)=
### Users

Any person who can log in to a Juju controller.

#### Juju administrator

A user with controller `superuser` access.

Owned by: Controller.

Used by: Controller.

Privileges: Full access.

Example threats:
- {ref}`threats-availability`: An administrator might accidentally or maliciously misconfigure critical components of the Juju environment, such as disabling key services or misallocating resources, leading to downtime or degraded service performance.
- {ref}`threats-confidentiality`: If an attacker gains access to the credentials of a Juju administrator, they can potentially access and control the entire Juju environment, including sensitive configuration data, user permissions, and operational commands. Or, a Juju user might inadvertently or maliciously expose sensitive data within the Juju model or applications. This could occur if the user accesses sensitive configuration details, logs, or secrets and then shares this information in an unsecured manner, such as storing it in an unprotected location or sending it over an insecure channel.
- {ref}`threats-integrity`: An attacker with access to an administrator's account might make unauthorized changes to the Juju configuration, such as altering model settings, modifying charm configurations, or changing user permissions.

Controls: {ref}`controls-user-authentication`, {ref}`controls-no-plaintext-passwords-in-the-database`

#### Juju user

A user with access less than controller `superuser` access.

Owned by: Controller.

Used by: Client.

Privileges: As permitted by their access level.

Example threats:

- {ref}`threats-confidentiality`: A Juju user might inadvertently or maliciously expose sensitive data within the Juju model or applications. This could occur if the user accesses sensitive configuration details, logs, or secrets and then shares this information in an unsecured manner, such as storing it in an unprotected location or sending it over an insecure channel.

Controls: {ref}`controls-user-authentication`,  {ref}`controls-granular-access`, {ref}`controls-no-plaintext-passwords-in-the-database`

(assets-workloads)=
### Workloads

The workloads installed and operated through charms.

Owned by: External entity.

Privileges: As defined by the external entity.

(security-data-flows)=
## Data flows

This section lists all the main data flows between assets in Juju, along with example threats and the types of controls Juju provides.

(data-flows-admin-user-logging-and-monitoring-systems)=
### Admin  user - Logging and monitoring systems

Involves the collection, aggregation, and presentation of operational data from the Juju environment. As the Juju environment runs, various components, such as agents (machine / container / unit) and the controller, generate logs and metrics related to system events, performance, and application states. These logs and metrics are collected and stored by the Juju logging and monitoring subsystems.

Example threats:
- {ref}`threats-availability`: An attacker could target the logging and monitoring system by generating excessive log entries or sending malformed data from compromised Juju agents. This could overwhelm the logging infrastructure, making it slow or unresponsive and resulting in a denial of service (DoS) condition.
- {ref}`threats-confidentiality`: If logs and monitoring data are transmitted from Juju agents or controllers to a logging and monitoring system without encryption or proper access controls, sensitive information such as usernames, IP addresses, API tokens, or configuration details could be exposed.
- {ref}`threats-integrity`: If the data flow between Juju administrators and logging/monitoring systems is not protected with integrity checks (e.g., using cryptographic hashes or signatures), an attacker could manipulate the logs or monitoring data in transit.

Controls: [TBA]

(data-flows-agent-controller)=
### Agent - Controller

Overview: When a user issues a command via the Juju client, it is processed by the controller's API server, which updates the central database. The agents on each machine, container, or unit then periodically poll the controller to retrieve the latest state from the database, ensuring their local state matches the desired state as defined by the controller.

Example threats:
- {ref}`threats-availability`: An attacker might flood the Juju API server with a large number of requests, causing it to become unresponsive to legitimate agent requests. This could prevent agents (machine / container / unit) from sending or receiving necessary data, leading to failure in performing tasks like configuration management, status updates, or lifecycle operations such as scaling or application removal.
- {ref}`threats-confidentiality`: If an attacker intercepts communication between an agent and the Juju controller, they could potentially capture sensitive data being transmitted, such as authentication tokens, configuration details, or secrets stored within the data flow. If communication is not encrypted using TLS, the attacker can read the data in transit, compromising its confidentiality.
- {ref}`threats-integrity`: An attacker intercepts and modifies API requests or responses between an agent and the Juju controller. For instance, modifying a request to deploy a specific application or configuration, resulting in unintended actions being performed on the target environment. Without proper cryptographic integrity checks, the controller might not detect that the data was altered.

Controls: {ref}`controls-agent-authentication`, {ref}`controls-rate-limiting`

(data-flows-agent-secret)=
### Agent - Secret

Overview: Involves secure interactions where an agent requests a secret from the controller via the Juju API whenever it needs sensitive information, such as passwords or API keys, to perform its tasks. The controller retrieves the secret from its secure storage or backend, ensures the agent has the necessary permissions, and securely delivers the secret to the agent.

Example threats:
- {ref}`threats-availability`: An attacker could launch a DoS attack against the Juju Secrets management system by overwhelming it with a high volume of requests from compromised or malicious Juju agents. This could make the secrets service unavailable to legitimate agents, causing applications to fail when they attempt to retrieve required secrets.
- {ref}`threats-confidentiality`: If the communication between Juju agents (machine / container / unit) and the Juju secrets management system is not properly secured (e.g., using weak encryption or no encryption at all), an attacker could intercept the network traffic and gain unauthorized access to sensitive secrets (e.g., API keys, database credentials, and certificates).
- {ref}`threats-integrity`: If there is no mechanism in place to verify the integrity of the data flow between Juju agents and secrets, an attacker who gains access to the communication channel could tamper with the secrets data. For example, an attacker could modify API keys, certificates, or configuration settings being fetched by an agent, causing the agent to use compromised or incorrect data, leading to a breach or operational failure.

Controls: {ref}`controls-tls-encryption`


(data-flows-agent-charms)=
### Agent - Charm

Overview: The agents (machine / container / unit) constantly monitor the state defined by the controller and the Juju database. When an agent detects that a particular action needs to be taken -- for example, if a new relation is added or a configuration change is required -- it triggers the appropriate event and invokes the corresponding hook in the charm.

Example threats:
- {ref}`threats-availability`: An attacker could exploit a vulnerability in the agent's reporting mechanism to flood a charm with excessive or malformed data. This could overwhelm the charm, causing it to slow down, become unresponsive, or crash. For example, a compromised unit agent could repeatedly send large amounts of log data or status updates to the charm, leading to a denial of service (DoS) condition
- {ref}`threats-confidentiality`: If data sent from Juju agents (machine / container / unit) to charms is not encrypted or properly protected, sensitive information such as operational metrics, log data, or configuration states could be intercepted by an attacker. For example, if a unit agent sends unencrypted logs containing sensitive information like access credentials or internal configurations to a charm, an attacker monitoring the network could capture and misuse this data.
- {ref}`threats-integrity`: If the communication from agents to charms lacks integrity checks (such as digital signatures or message digests), an attacker could intercept and modify the data in transit. For instance, an attacker could alter the operational metrics or status information being sent from a unit agent to a charm.

Controls: [TBA]

(data-flows-client-controller)=
### Client - Controller

Overview: These clients issue commands to manage and manipulate the state of the Juju environment, such as deploying applications, managing units, or configuring models. The controller's API server processes these commands, validating them, and updating the system's state accordingly. The controller then communicates any necessary changes to the Juju database and coordinates with agents across the environment. The API server also provides feedback to the clients, such as operation statuses or errors, ensuring that the client is aware of the outcomes of its requests.

Example threats:
- {ref}`threats-confidentiality`: If clients communicate with the controller / API server over an unsecured channel (e.g., HTTP instead of HTTPS), an attacker could intercept the credentials (such as API tokens, passwords, etc.) being sent from the client to the API server. This could lead to unauthorized access to the Juju environment, allowing the attacker to perform malicious operations or gain access to sensitive information.
- {ref}`threats-integrity`: If the data sent from clients to the controller / API server is not properly sanitized or validated, an attacker could inject malicious commands or data into the API calls. For example, by manipulating Python scripts, CLI commands, or Terraform configurations, an attacker could craft requests that alter the system state, modify deployment configurations, or execute unauthorized commands on the Juju environment.

Controls: {ref}`controls-user-authentication`

(data-flows-client-simplestreams)=
### Client - Simplestreams

Overview: When a client initiates an operation like bootstrapping a controller, it needs to fetch necessary resources such as cloud images, agent binaries, or charm metadata.

Example threats:
- {ref}`threats-availability`: An attacker could perform a DoS attack by sending a high volume of requests from compromised or malicious clients (e.g., scripted attacks using Python or Terraform). This could overwhelm streams.canonical.com, making it unavailable to legitimate users.
- {ref}`threats-confidentiality`: If a client retrieves images, charms, or other resources from streams.canonical.com over an unencrypted channel (e.g., HTTP instead of HTTPS), an attacker could intercept the data.
- {ref}`threats-integrity`: If the data flow between the clients and streams.canonical.com does not include integrity verification (e.g., checksums or digital signatures), an attacker could intercept and modify the data in transit.

Controls: {ref}`controls-tls-encryption`

(data-flows-controller-blob-storage)=
### Controller - Blob storage

Overview: The data flow between the Juju controller / API server and blob storage involves the controller accessing and managing binary large object (blob) storage to handle charm resources, backups, and other large data objects. When deploying applications, the controller may need to store or retrieve charm resources, such as files or dependencies defined in the charm.

Example threats:
- {ref}`threats-availability`: An attacker could target the blog storage service with a DoS attack by overwhelming it with excessive requests, potentially originating from the controller / API server or other compromised systems.
- {ref}`threats-confidentiality`: If the Juju controller / API server interacts with a blog storage service to store or retrieve content (e.g., for documentation, user-generated content, or logs), and the communication is not encrypted, an attacker could intercept the data.
- {ref}`threats-integrity`: If the data flow between the controller / API server and the blog storage service does not include integrity checks (e.g., using digital signatures, checksums, or message digests), an attacker could manipulate the content in transit.

Controls: {ref}`controls-tls-encryption`

(data-flows-controller-cloud-provider)=
### Controller - Cloud provider

Overview: Involves the controller interacting directly with the cloud provider's API to manage resources such as virtual machines, storage, and networking. When a Juju user issues commands to deploy applications or manage infrastructure, the controller translates these commands into specific API requests to the cloud provider.

Example threats:
- {ref}`threats-availability`: An attacker could exploit this dependency by launching a Denial of Service (DoS) attack on the cloud provider's API endpoints.

Controls: {ref}`controls-tls-encryption`

(data-flows-controller-container-registry)=
### Controller - Container registry

Overview: The data flow involves the controller interacting with the registry to manage and deploy container-based workloads. When a Juju model or application is deployed that requires container images, the controller pulls the necessary images from a container registry, such as Docker Hub, Canonical’s Container Registry, or a private registry.

Example threats:
- {ref}`threats-availability`: An attacker could target the communication between the controller / API server and the container registry by sending a high volume of requests or malformed requests.
- {ref}`threats-integrity`: If the data flow between the controller / API server and the container registry does not implement strong integrity verification mechanisms (such as digital signatures or checksums for images), an attacker could intercept and tamper with the images in transit.

Controls: {ref}`controls-tls-encryption`

(data-flows-controller-controller)=
### Controller - Controller

Overview: The data flow between Juju controllers (controller / API server to controller / API server) primarily involves communication for high availability (HA) setups, cross-model relations, and multi-cloud operations. In an HA configuration, multiple controllers communicate with each other to maintain consistency and coordination. They share information about the cluster's state, synchronize changes to the Juju database, and elect a leader to manage decision-making and API requests.

For cross-model relations or when offering and consuming services between different models hosted on separate controllers, these controllers exchange API calls to establish and maintain relationships, ensuring that the service data and operational commands are correctly propagated and synchronized.

Example threats:
- {ref}`threats-availability`: An attacker could exploit a vulnerability or create network congestion between controllers, disrupting the communication channels needed for leader election, state synchronization, or operational updates.
- {ref}`threats-confidentiality`: When multiple Juju controllers communicate with each other (e.g., in a high-availability (HA) setup for state synchronization or leader election), if the communication is not encrypted (e.g., using HTTP instead of HTTPS or an insecure channel), an attacker could intercept the data.
- {ref}`threats-integrity`: If the communication between Juju controllers does not have proper integrity checks (e.g., digital signatures, message digests), an attacker could intercept and modify the data being transmitted.

Controls: {ref}`controls-tls-encryption`

(data-flows-controller-database)=
### Controller - Database

Overview: When the controller receives commands or updates from the Juju client, it processes these requests and writes the necessary changes to the Juju database. The database acts as the single source of truth, storing all state information about the models, machines, containers, and units. The controller continuously interacts with the database, reading the current state, processing updates, and writing any changes.

Example threats:
- {ref}`threats-availability`: An attacker could overwhelm the network or database service with excessive requests, causing legitimate requests from the controller / API server to the Juju DB to be delayed or dropped. This would make the Juju DB unavailable.
- {ref}`threats-confidentiality`: If the communication between the controller / API server and Juju DB is not encrypted properly, an attacker could intercept sensitive information. For instance, an attacker could listen to the network traffic and capture sensitive data such as database credentials, configuration details, or operational commands.
- {ref}`threats-integrity`: An attacker who gains access to the communication channel between the controller / API server and Juju DB could alter the data being transmitted. This could involve modifying commands or data results, leading to incorrect operations, misconfigurations, or corrupt state in Juju DB.

Controls: {ref}`controls-database-authentication`

(data-flows-simplestreams)=
### Controller - Simplestreams

Overview: Involves the controller accessing and downloading metadata and resources required for managing Juju models and deploying applications. streams.canonical.com hosts important data such as agent binaries, cloud images, and charm metadata, which the Juju controller needs to operate efficiently.

Example threats:
- {ref}`threats-availability`: An attacker could attempt to overwhelm streams.canonical.com or the Juju controller / API server with excessive or malformed requests, leading to service unavailability.
- {ref}`threats-confidentiality`: When a Juju controller / API server interacts with streams.canonical.com to fetch images, charms, or other resources, if the communication channel is not encrypted (e.g., using plain HTTP instead of HTTPS), an attacker could intercept the data.
- {ref}`threats-integrity`: If the data flow between the controller / API server and streams.canonical.com lacks integrity verification (such as cryptographic hashes or digital signatures), an attacker could perform a Man-in-the-Middle (MitM) attack.

Controls: {ref}`controls-tls-encryption`

(data-flows-database-database)=
### Database - Database

Overview: The data flow between Juju databases occurs in high-availability (HA) configurations where multiple instances of the Juju controller are set up to ensure redundancy and fault tolerance. In an HA setup, each Juju controller has its own instance of the Juju database (usually backed by MongoDB or, more recently, Dqlite) that needs to stay synchronized with the others.

Example threats:
- {ref}`threats-availability`: An attacker could overwhelm one or both Juju DB instances with excessive or malformed synchronization requests, effectively causing a DoS condition.
- {ref}`threats-confidentiality`: If the communication between Juju DB instances is not encrypted (e.g., using plain TCP connections instead of TLS), an attacker could intercept the data being replicated or synchronized between the databases.
- {ref}`threats-integrity`: Without proper integrity checks (e.g., digital signatures, hashes, or cryptographic checksums), an attacker could perform a MitM attack, intercepting and modifying the data being transferred between Juju DB instances.

Controls: {ref}`controls-tls-encryption`

(data-flows-ssh-key-or-credential-agent)=
### SSH key or credential - Agent

Overview: Involves securely provisioning and managing access to machines and units. When a machine or unit is provisioned, the Juju controller injects the necessary SSH keys and credentials into the agent managing that machine or unit. This allows the agent to securely execute operations, perform updates, and manage configurations on the machine or container.

Example threats:
- {ref}`threats-availability`: An attacker could perform a DoS attack by intercepting and corrupting the transmission of SSH keys and credentials, causing the agents to fail in receiving the necessary authentication materials. This could prevent agents from accessing critical resources or performing operations that require SSH access, effectively disrupting the availability of services managed by the agents.
- {ref}`threats-confidentiality`: If the transfer of SSH keys and credentials to Juju agents (machine / container / unit) is not encrypted or properly secured, an attacker could intercept the transmission and gain access to sensitive authentication materials. For example, if an attacker captures SSH private keys or passwords in transit, they could use them to gain unauthorized access to the machines or containers managed by Juju agents.
- {ref}`threats-integrity`: If the data flow of SSH keys and credentials to the agents is not protected with integrity checks (e.g., digital signatures, checksums), an attacker could modify the data in transit. For instance, an attacker could intercept the transmission and replace the legitimate SSH key with a malicious one, allowing them to gain unauthorized access to the agent-managed systems or impersonate legitimate users.

Controls: {ref}`controls-filesystem-permissions`

(security-threats-details)=
## Threats -- detail

This section defines the threat types mentioned for the assets and data flows above.

(threats-availability)=
### Availability

A threat to availability is any situation that prevents access to your data (aka 'Denial-of-Service (DoS) attack').

This could be due to malicious actors or to improper configuration.

(threats-confidentiality)=
### Confidentiality

A threat to confidentiality is any situation where an unauthorized entity can view your data.

This could be due to malicious actors but also to accidental leaks.

(threats-integrity)=
### Integrity

A threat to integrity is any situation where an entity tampers with your data, whether by mistake or intentionally.

## Controls -- detail

This section defines the controls mentioned for the assets and data flows above.

(controls-agent-authentication)=
### Agent authentication

Any Juju agent interacting with a Juju controller is authenticated with a password.

(controls-observability)=
###  Auditing and logging

Juju offers auditing and logging capabilities to help administrators track user activities, changes in the environment, and potential security incidents. These logs can be useful for identifying and responding to security threats or compliance requirements.

```{ibnote}
See more: {ref}`Logs <log>`
```

(controls-charm-development-best-practices)=
### Charm development best practices

Charm SDK developers support charm authors with documentation on best practices.

While any charm can be published on Charmhub, only charms that have passed formal review will be publicly listed in search results.

```{ibnote}
See more: [Ops | Publish your charm](https://documentation.ubuntu.com/ops/latest/howto/manage-charms/#publish-your-charm) and references therein.
```

(controls-database-authentication)=
### Database authentication

Any controllers, agents, or administrators trying to access the database must authenticate.

(controls-filesystem-permissions)=
### Filesystem permissions

Juju restricts filesystem permissions following a minimum access policy.

(controls-granular-access)=
### Granular access

Juju does not currently have RBAC or ReBAC access. However, you can restrict user access at the controller, cloud, model, and application offer level.

```{ibnote}
See more: {ref}`user-access-levels`
```

For more control, you can use JAAS, which does support ReBAC authorization.

```{ibnote}
See more: [JAAS | Authorization](https://documentation.ubuntu.com/jaas/v3/explanation/jaas-authorization/)

```

(controls-high-availability)=
### High availability

A controller on a machine cloud can operate in high availability mode. Depending on the charm, a charmed application on either a machine or a Kubernetes cloud can operate in high availability mode as well.

```{ibnote}
See more: {ref}`high-availability`
```

(no-cloud-credentials-in-juju)
### No cloud credentials in Juju

In a typical Juju workflow you allow your client to read your locally stored cloud credentials, then copy them to the controller, so that the controller can use them to authenticate with the cloud. However, for some clouds, Juju now supports a workflow where  neither your client nor your controller know your credentials directly -- you can just supply an instance profile (AWS) or a managed identity (Azure).

```{ibnote}
See more: {ref}`bootstrap-a-controller`, {ref}`cloud-ec2`, {ref}`cloud-azure`
```

(controls-no-plaintext-passwords-in-the-database)=
### No plaintext passwords in the database

All passwords in the database are hashed and salted.

(controls-no-sensitive-information-in-logs)=
### No sensitive information in logs

Ensures sensitive information isn't revealed in Juju logs.

(controls-rate-limiting)=
### Rate limiting

Authentication requests from a Juju unit agent to a Juju controller are rate-limited.

(controls-regular-backups)=
### Regular backups

For machine controllers, Juju provides tools to help with controller backups. This can help restore healthy state in the case of an attack affecting data integrity.

(controls-regular-updates)=
### Regular updates

Canonical releases updates and security patches for Juju to address vulnerabilities, improve performance, and add new features.

```{ibnote}
See more: {ref}`juju-roadmap-and-releases`
```

(controls-rootless-charms)=
### Rootless charms

Kubernetes charms can be set up to not require root access.

```{ibnote}
See more: [Charmcraft | File `charmcraft.yaml` > `charm-user`](https://documentation.ubuntu.com/charmcraft/stable/reference/files/charmcraft-yaml-file/#charm-user), [Charmcraft | File `charmcraft.yaml` > `containers`](https://documentation.ubuntu.com/charmcraft/stable/reference/files/charmcraft-yaml-file/#containers)
```

(controls-secret-backends-can-be-external)=
### Secret backends can be external

Juju ensures secrets can be stored in an external secret backend, with additional layers of control.

Juju follows the industry standard for secret backends and supports Hashicorp Vault.

```{ibnote}
See more: {ref}`Secret backends <secret-backend>`
```

(controls-secrets)=
### Secrets

Juju ensures applications can track separate items that they consider high value with elevated security, as secrets.

```{ibnote}
See more: {ref}`Secrets <secret>`
```

(controls-time-limited-tokesn)=
### Time-limited tokens

Macaroons are time-limited.

(controls-tls-encryption)=
### TLS encryption

Any communication to and from a Juju controller’s API server and clients, Charmhub, the container registry, the cloud image registry, clouds, or the application units deployed with their help,  is TLS-encrypted (using AES 256).

(controls-user-authentication)=
### User authentication

User authentication with the controller, machines provisioned by the controller, the controller database, etc., is implemented following industry standards. That is:

* macaroons
* (for Juju with [JAAS](https://jaas.ai/); added in Juju 3.5) JWTs
* SSH keys
* passwords


(controls-vpn)=
### Virtual Private Network (VPN)

Supports running the controllers on an isolated network that does not have direct public access.
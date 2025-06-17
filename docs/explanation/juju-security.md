(juju-security)=
# Juju security

> See also: {ref}`harden-your-deployment`

Malicious actors may try to prevent you from accessing your data (Denial-of-Service (DoS) attacks, affecting availability); view your data (attacks affecting confidentiality); or tamper with your data (Man-in-the-Middle attacks, affecting data integrity). Juju takes a variety of means to protect you against all of these.

##  TLS-encrypted communication

Any communication to and from a Juju controller’s API server and clients, Charmhub, the container registry, the cloud image registry, clouds, or the application units deployed with their help,  is TLS-encrypted (using AES 256).

> See more: [Wikipedia | TLS](https://en.wikipedia.org/wiki/Transport_Layer_Security)

## User authentication

User authentication with the controller, machines provisioned by the controller, the controller database, etc., is implemented following industry standards. That is:

* macaroons
* (for Juju with [JAAS](https://jaas.ai/); added in Juju 3.5) JWTs
* SSH keys
* passwords

## Role-based access

Juju does not currently have role-based access. However, you can restrict user access at the controller, cloud, model, and application offer level.

> See more: {ref}`User access levels <user-access-levels>`

## Agent authentication

Any Juju agent interacting with a Juju controller is authenticated with a password.

## Rate limiting

Authentication requests from a Juju unit agent to a Juju controller are rate-limited.

## Database authentication

Any controllers, agents, or administrators trying to access the database must authenticate.

## No plaintext passwords in the database

All passwords in the database are hashed and salted.

## High availability

A controller on a machine cloud can operate in high availability mode. Depending on the charm, a charmed application on either a machine or a Kubernetes cloud can operate in high availability mode as well.

## Filesystem permissions

Juju restricts filesystem permissions following a minimum access policy.

## Regular backups

For machine controllers, Juju also provides tools to help with controller backups. This can help restore healthy state in the case of an attack affecting data integrity.

## Time-limited tokens

Macaroons are time-limited.

## Secrets and secret backends

Charmed applications can track high-value configurations as secrets.

Juju follows the industry standard for secret backends and supports Hashicorp Vault.

> See more: {ref}`Secret <secret>`, {ref}`Secret backends <secret-backend>`


## No sensitive information in logs

Juju is careful not to store sensitive information in logs.

> See more: {ref}`Logs <log>`

##  Auditing and logging

Juju offers auditing and logging capabilities to help administrators track user activities, changes in the environment, and potential security incidents. These logs can be useful for identifying and responding to security threats or compliance requirements.

> See more: {ref}`Logs <log>`

## Guided, tested, and maintained operations code

Juju encourages developers to follow best practices in creating software operators ('charms'). This includes secure coding guidelines, testing, and regular maintenance to address potential security vulnerabilities.

## Regular updates and patches

Canonical releases updates and security patches for Juju to address vulnerabilities, improve performance, and add new features.

> See more: {ref}`juju-roadmap-and-releases`

# Test cases

This document lists and describes a collection of tests to evaluate the correctness of the operations offered by juju from a users perspective. The table below summarizes theses tests and its current status.


Name | Path | Last Rev | Working |
---------|----------|--------- | --------- |
Applications for machine charms |
[Single deploy](#Single-deploy) | path | last-rev | :heavy_check_mark: |
[Multiple deployments](#multiple-deployments) | path | last-rev |  :heavy_check_mark: |
[Deploy to a specific machine](#deploy-to-a-specific-machine) | | | |
[Deploy to a specific availability zone](#deploy-to-a-specific-availability-zone) | | | |
| Applications for k8s charms| | |


# Applications for machine charms

Collection of tests for applications using machine charms.

## Single deploy

* Deploy `apache2`
* Expose it
* Use `curl` to check it is up and running
* Deploy again to find a failure
* Destroy the app

## Multiple deployments

* Deploy `apache2`
* Expose `apache2`
* Deploy `byobu-classroom`
* Expose `byobu-classroom`
* Use `curl` to check `apache2` is up and running
* Use `curl` to check `byobu-classroom`is up and running
* Destroy `apache2`
* Destroy `byobu-classroom`

## Deploy to a specific machine

* Add a new unit
* Deploy `apache2` to the new unit
* Expose the application
* Use `curl` to check `apache2` is up and running
* Destroy `apache2`
* Destroy the new unit

## Deploy to a specific availability zone

* Deploy `apache2` to the availability zone 2
* Expose it
* Use `curl` to check it is up and running
* Destroy the app

## Deploy to a network space

* Deploy `apache2` to the network space 2
* Expose it
* Check IP address corresponds to network space 2
* Use `curl` to check it is up and running
* Destroy the app

## Trust an application with a credential

* TBD

## Refresh application

* Deploy `apache2` latest/candidate
* Wait for deployment to be complete
* Expose it
* Use `curl` to check it is up and running
* Refresh `ubuntu` charm to be latest/stable
* Use `curl` to check it is up and running
* Destroy the app

## Remove application

* Deploy `ubuntu`
* Remove the application
* Wait for completion
* Check it was removed
* Attempt to remove again
* Check it failed

# Models

Use cases regarding models.

## Add/Remove a model
* Add model `toremove`
* Check it exists
* Remove model
* Check it was removed

## Add/Remove/Switch models
* Add model `toremove`
* Check it exists
* Switch to default model
* Check swith was done
* Destroy `toremove` model
* Check it was removed
* Attempt to switch 

## Configure a model
* Add model `daily` with `--config image-stream=daily`
* 

## Upgrade model
## Migrate a model
## Work with multiple users
## List models

# Relations
## Manage relations
## Cross-model relations

# Machine
## Add a machines
## Upgrade machine series

# Storage
TBD

# Applications for K8s charms

Collection of tests for K8s-based charms.
 
## K8s Single deploy
## K8s Multiple deployment
## K8s Deploy to a specific machine
## K8s Deploy to a specific availability zone
## K8s Deplo y to a network space
## K8s Trust an application with a credential
## K8s Refresh application
## K8s Remove application


# K8s Models
## K8s Add/Remove a model
## K8s Add/Remove/Switch models
## K8s Configure a model
## K8s Upgrade model
## K8s Migrate a model
## K8s Work with multiple users

# K8s Relations
## K8s Manage relations
## K8s Cross-model relations

# Machine
## Upgrade machine series
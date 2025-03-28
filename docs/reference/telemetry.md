(telemetry)=
# Telemetry

Telemetry is the automatic recording and transmission of data from remote sources. In Juju, it specifically refers to the gathering of routine business metrics with the purpose of helping developers improve Juju. This happens automatically once a day, on a per model basis.  For more details, see below.

```{note}

No user information is gathered.

```

## What data is collected?

* For a controller
  * juju version
  * controller uuid
* For a model
  * number of applications
  * number of units deployed
  * number of machines deployed
  * cloud
  * cloud provider
  * cloud region
  * model uuid
* For a charm
  * number of units
  * names of charms the charm is related to

No user information is gathered.

## What is the data used for?


The data will help us gain a better understanding of how Juju is used in the field:

* Are most configurations using high availability for the controller?
* Are many models with few units more popular than few models with many units?
* How many models are used with a controller?
* Which clouds are the most popular?
* What charms are frequently used together?

For example, we can better design improvements, create new bundles for charms commonly used together, etc.

Eventually, some data, such as the names of the charms an application is related to, will also be available to charm authors for use in improving their charms. 

## How do I disable data collection?

To disable telemetry in a juju model, set the `disable-telemetry` model configuration key to `true`:

```text
juju model-config disable-telemetry=true
```

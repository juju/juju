(charming-history)=
# About charming history

Since its beginnings in 2009, the writing of charms has gone through multiple phases, comprising three different frameworks based on three different libraries. This document explains this evolution.

## Background

Juju was initially conceived in 2009 by Mark Shuttleworth, Gustavo Niemeyer and Simon Wardley [[1](https://blog.labix.org/2013/06/25/the-heart-of-juju)] as a way to simplify the deployment and operation of applications and their supporting services. Juju enables users to both graphically and non-graphically model complex deployments which are composed using one or more Charms - reusable packages that contain all the instructions necessary to deploy, configure, operate and integrate software.

The Juju team adopted the Go language early-on, noting its suitability to cloud-first environments. This is an observation that has been made by numerous subsequent cloud-native projects - [Kubernetes](https://kubernetes.io/), [Terraform](https://terraform.io), [containerd](https://containerd.io/) to name just a few.

Juju has evolved significantly over time, and throughout its life a number of frameworks and libraries have been authored to simplify and standardise the way Charms are written. One thing that hasn't changed throughout time however, is the approach that Juju takes to operating workloads - Juju's fundamentally works just like it did in 2009, but now supports many more underlying compute platforms (such as Kubernetes).

Charms can be written in any language - that was as true in 2009 as it is today. Over time however, the Charming community has settled on Python as its language of choice. Python is extremely popular, has a huge community and is already commonly used to perform system automation tasks, making it a great choice for authoring Charms and for ensuring re-usability among the charming community.

## 2014: The Services Framework


The Services Framework evolved from the [charmhelpers](https://github.com/juju/charm-helpers) library; it aimed to standardise an approach to event handling and Charm structure. The services framework provided the first steps toward more declarative operators, and more consistency across the charm landscape.

## 2015: The Reactive Framework


Derived from [reactive programming](https://en.wikipedia.org/wiki/Reactive_programming), the [charms.reactive](https://charmsreactive.readthedocs.io/) library was first announced on the [Ubuntu Blog](https://ubuntu.com/blog/charming-2-0-now-with-100-more-awesome). It enabled developers to continue authoring Charm actions in the familiar “hook contexts” that were fundamental to the Services Framework, but favoured an event driven approach that simplified the development of charms. The Reactive framework also saw the introduction of *layers*, which could be reused and composed into new charms.

## 2019 - Present: The Operator Framework Ops

The latest and most current framework is the [Ops framework](https://github.com/canonical/operator), which was released with initial focus on enabling the development of charms for Kubernetes, while also simplifying the development of charms for other substrates. This framework provides a single library for developers to target when authoring charms for any substrate. The Ops Framework extends the operator pattern beyond Kubernetes and into multi-cloud, multi-substrate application management.

The Ops framework is event driven and implements the [observer pattern](https://en.wikipedia.org/wiki/Observer_pattern). The Juju controller emits events that charms observe and respond to at key points during an application’s {ref}`lifecycle <hook>`. In keeping with the goal of enabling developers to share and reuse quality, reviewed operator code, the Ops Framework introduced the concept of charm [libraries](https://canonical-charmcraft.readthedocs-hosted.com/stable/reference/files/libname-py-file/).

In addition to the new framework, a new tool was introduced named [`charmcraft`](https://github.com/canonical/charmcraft), which enables developers to easily create new charms (templated for use with the Ops Framework), and publish/release charms to the [Charmhub](https://charmhub.io) - the home of the Open Operator Collection.

The Ops Framework, in combination with Charmcraft, is the recommended way to write charms now. However, charms previously written using other frameworks and libraries will continue to work.

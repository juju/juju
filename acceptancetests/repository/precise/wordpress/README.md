# Overview

WordPress is a powerful blogging platform written in PHP. This charm aims to deploy WordPress in a fashion that will allow anyone to scale and grow out
a single installation.

# Usage

This charm is available in the Juju Charm Store, to deploy you'll need at a minimum: a cloud environment, a working Juju installation,
and a successful bootstrap. Please refer to the [Juju Getting Started](https://juju.ubuntu.com/docs/getting-started.html) documentation before continuing.

Once bootstrapped, deploy the MySQL charm then this WordPress charm:

    juju deploy mysql
    juju deploy wordpress

Add a relation between the two of them

    juju add-relation wordpress mysql

Expose the WordPress installation

    juju expose wordpress

## Scaled Down Usage for Personal Use

If you're just looking to run a personal blog and want to save money you can run all of this on a single node, here's an entire single node installation from scratch: 

    juju bootstrap
    juju deploy --to 0 wordpress
    juju deploy --to 0 mysql
    juju add-relation wordpress mysql 
    juju expose wordpress

This will run everything on one node, however we still have the flexibility to grow horizontally. If your blog gets more traffic and you need to scale:

    juju add-unit wordpress

Since we're omitting the `--to` command Juju will fire up a new dedicated machine for Wordpress and relate it. You can also `remove-unit` when the surge is over and go back to a cheaper one node set up. 

# Scale Out Usage 

You can deploy a memcached server and relate it to your WordPress service to add memcache caching. This will 
automagically install [WP-FFPC](http://wordpress.org/extend/plugins/wp-ffpc/) (regardless of your tuning settings) and configure it to cache 
rendered pages to the memcache server. In addition to this layer of caching, Nginx will pull directly from memcached bypassing PHP altogether. 
You could theoretically then turn off php5-fpm on all of your servers and just have Nginx serve static content via memcached (though, you 
wouldn't be able to access the admin panel or any uncached pages - it's just a potential scenario).

    juju deploy memcached
    juju add-relation memcached wordpress
    
This setup will also synchronize the flushing of cache across all WordPress nodes, making it ideal to avoid stale caches.

A small note, when using the Apache2 engine and memcache, all request will still be sent to WordPress via Apache where typical caching 
procedures will take place and wp-ffpc will render the memcached page.

# Configuration

This WordPress charm comes with several tuning levels designed to encompass the different styles in which this charm will be used.

A use case for each tuning style is outlined below:

## Bare

The Bare configuration option is meant for those who wish to run the stock WordPress setup with no caching, no manipulation of data, 
and no additional scale out features enabled. This is ideal if you intend to install additional plugins to deal with coordinating
WordPress units or simply wish to test drive WordPress as it is out of the box. This will still create a load-balancer when an additional
unit is created, though everything else will be turned off (WordPress caching, APC OpCode caching, and NFS file sharing).

To run this WordPress charm under a bare tuning level execute the following:

    juju set wordpress tuning=bare

## Single

When running in Single mode, this charm will make every attempt to provide a solid base for your WordPress install. By running in single
the following will be enabled: Nginx microcache, APC OpCode caching, WordPress caching module, and the ability to sync files via NFS.
While Single mode is designed to allow for scaling out, it's meant to only scale out for temporary relief; say in the event of a large
traffic in-flux. It's recommended for long running scaled out versions that optimized is used. The removal of the file share speeds up
the site and servers ensuring that the most efficient set up is provided. 

To run this WordPress charm under a single tuning level execute the following:

    juju set wordpress tuning=single

## Optimized

If you need to run WordPress on more than one instance constantly, or require scaling out and in on a regular basis, then Optimized is the
recommended configuration. When you run WordPress under an Optimized tuning level, the ability to install, edit, and upgrade themes and plugins
is disabled. By doing this the charm can drop the need for an NFS mount which is inefficient and serve everything from it's local disk.
Everything else provided in Single level is available. In order to install or modify plugins with this setup you'll need to edit and commit
them to a forked version of the charm in the files/wordpress/ directory.

To run this WordPress charm under an optimized tuning level execute the following:

    juju set wordpress tuning=optimized

### Handling wp-content

In order to allow for custom WordPress content within the Juju charm a separate configuration option exists for pointing to any Git or Bzr 
repository. An example of a valid formed wp-content repository can be found on the [Juju Tools Github page](https://github.com/jujutools/wordpress-site). 
To set the wp-content directive to a git repository, use one of the following formats making sure to replace items like `host`, `path`, and `repo` with their 
respective names:

    juju set wordpress wp-content=git@host:path/repo.git

or

    juju set wordpress wp-content=http://host/path/repo.git
    
or

    juju set wordpress wp-content=git://host/path/repo.git
    
If you wish to use a bzr repository, then apply one of the following schemes replacing items like `host`, `username`, `path`, and `repo` with their respective values:

For LaunchPad hosted repostiories:

    juju set wordpress wp-content=lp:~username/path/repo
    
For other Bzr repositories:

    juju set wordpress wp-content=bzr://host/path/repo

or

    juju set wordpress wp-content=bzr+ssh://host/path/repo
    
Setting the wp-content option to an empty string ("") will result in no further updates being pulled from that repository; however, the last pull will remain 
on the system and will not be removed.

## debug

This option will create a directory `_debug` at the root of each unit (`http://unit-address/_debug`). In this directory are two scripts: info.php (`/_debug/info.php`) 
and apc.php (`/_debug/apc.php`). info.php is a simple phpinfo script that will outline exactly how the environment is configured. apc.php is the APC admin portal which 
provides APC caching details in addition to several administrative functions like clearing the APC cache. This should never be set to "yes" in production as it exposes 
detailed information about the environments and may provide a way for an intruder to DDoS the machine.

    juju set wordpress debug=yes

to disable debugging:

    juju set wordpress debug=no

The debugging is disabled by default.

## Engine

By default the WordPress charm will install nginx and php5-fpm to serve pages. In the event you do not wish to use nginx - for whatever reason - you can switch to Apache2.
This will provide a near identical workflow as if you were using nginx with one key difference: memcached. In nginx, the cached pages are served from memcached prior to
hitting the php contents, this isn't possible with apache2. As such memcached support still works, since it falls back to the WordPress caching engine, but it's not as robust.
Otherwise, Apache2 will still perform balancing and everything else mentioned above. You can switch between engines at will with the following:

    juju set wordpress engine=apache2

Then back to nginx:

    juju set wordpress engine=nginx

Any other value will result in the default (nginx) being used.

# Known Limitations and Issues

## HP Cloud 

At this time WordPress + Memcached don't work on HP Cloud's standard.xsmall. To get around this deploy the WordPress charm with the 
charm to at least a `standard.small`, to do this:

    juju deploy --constraints "instance-type=standard.small" wordpress

This only is a problem when attempting to relate memcached to WordPress, otherwise an xsmall is _okay_ though it's really not the best 
sized platform for running a stable WordPress install.

## Single mode and the scale-out

If you're in Single mode and you want to/need to scale out, but you've been upgrading, modifying, and installing plugins + themes like
a normal WordPress user on a normal install; you can still scale out but you'll need to deploy a shared-fs charm first. At the time of
this writing only the NFS charm will work, but as more shared-fs charms come out (gluster, ceph, etc) that provide a shared-fs/mount 
interface those should all work as well. In this example we'll use NFS:

    juju deploy nfs
    juju add-relation nfs wordpress:nfs

By doing so, everything in the wp-contents directory is moved to this NFS mount and then shared to all future WordPress units. It's strongly
recommended that you first deploy the nfs mount, _then_ scale WordPress out. Failure to do so may result in data loss. Once nfs is deployed, 
running, and related you can scale out the WordPress unit using the following command:

    juju add-unit wordpress
    
In the event you want more than one unit at a time (and do not wish to run the add-unit command multiple times) you can supply a `-n` number
of units to add, so to add three more units:

    juju add-unit -n3 wordpress

# Contact Information

## WordPress Contact Information

- [WordPress Homepage](http://www.wordpress.org)
- [Reporting bugs](http://codex.wordpress.org/Reporting_Bugs) on WordPress itself
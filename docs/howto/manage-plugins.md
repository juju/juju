(manage-plugins)=
# How to manage plugins

> See also:  {ref}`plugin`

Juju can be easily extended by using plugins. There are many third-party plugins available to simplify certain tasks. However, you can easily create your own custom plugins for your specific needs. This tutorial will show you how to do this. 

```{important}

Another way to integrate Juju into your own tooling is to use the [Python](https://github.com/juju/python-libjuju) and [JavaScript](https://github.com/juju/js-libjuju) client libraries.

```

## Create a plugin

Let's say we want to get the IP address for a machine. To do this, we can run `juju status`, look at the table of units, find the row for the desired machine and read off the IP address. However:
 - we cannot easily do this inside a Bash script
 - this might get tedious if we have to do it repeatedly.

To solve this, we can write a plugin that, given the ID for a machine, will return the IP address. We want to be able to run it like this:
```console
$ juju ip 0
10.50.10.50
```

Let's get started. Create a file called `juju-ip` with the following contents:

```bash
#!/bin/bash

# First arg is machine ID
MACHINE=$1
# The jq query we will use to find the IP address
QUERY=$(printf '.machines."%d"."dns-name"' "$MACHINE")
# Call juju status, use jq to filter output
juju status --format=json | jq -r "$QUERY"
```

This is a simple Bash script that calls `juju status` using JSON output, so that we can use `jq` to filter the output and get the IP address. (You'll need to install [`jq`](https://stedolan.github.io/jq/) for this to work.)

```{important}
We've used Bash for this plugin, but you can use any language which allows command-line execution. Feel free to use Python, JS, Go, etc - whatever you are most familiar with.
```


## Install and run a plugin


To run this plugin, we just need to ensure that the `juju-ip` file is placed somewhere on our `$PATH`. In a terminal, run
```
echo $PATH | tr ':' '\n'
```
This will output all the directories on your `$PATH`. Put the `juju-ip` file in any one of these directories.

We also need to ensure our plugin is an executable file. In your terminal, run
```
sudo chmod +x [path/to/juju-ip]
```
replacing `[path/to/juju-ip]` with the new location of the `juju-ip` file.

Now, we are ready to run the plugin! Switch to a model with some machines deployed, and run
```
juju ip 0
```

> When you run
> ```
> juju [command] [args]
> ```
> and `[command]` is **not** a built-in Juju command, Juju will automatically search your `$PATH` for a file named `juju-[command]`, and attempt to execute it with the given `[args]`. This mechanism is what makes plugins possible.


## Distribute a plugin

If you've written something that you think other Juju users might find useful, feel free to distribute it. We suggest:
- creating a GitHub repo to host your plugin
- making a post about on the [Juju Discourse](https://discourse.charmhub.io/) forum
- adding your plugin to our {ref}`list <list-of-known-juju-plugins>` of third-party Juju plugins.

<!--
NB: these instructions are far too detailed I think.

There is no set format for distributing your plugin. However, for plugins that are intended to be widely available, the process is usually as follows:

**Before publishing**

You've coded up your plugin idea. That's great. Now you need to pause and check that you're not going to get yourself or anyone else into trouble by installing it! 

- Create an account on the [Juju Discourse](https://discourse.charmhub.io/). It's the primary medium for the community to interact. 
- Ensure that you are legally entitled to release the code. If you've developed the plugin at work, your employer may be the copyright holder.
- Do a final check and attempt to install Juju and the plugin on a fresh machine. Your installation instructions should "just work".
 
**Alpha/beta quality releases**

At this stage, you want to receive feedback and add polish. 

- Post a note on Discourse inviting people to be beta testers.
- Where relevant, upload the plugin to a relevant package manager.
- Make sure that your project has a complete README file with installation instructions and dependency requirements.


**General release**

Your plugin is mature and should now be fully functional. To announce that it is generally available, make it simple to find and install.

- Submit a pull request to have it included in the [plugins Github repository](https://github.com/juju/plugins). Also add it to the {ref}`List of known `juju` plugins <list-of-known-juju-plugins>`.
- Consider making the plugin installable via `snap`. This will enable everyone on Linux to install it, regardless of their local environment.

**Promotion**

Your plugin is doing well and your users are happy. Great work. Growing your userbase will mean that more lives can be simplified.

- Add a Reference doc and a How-to guide doc on [Juju Discourse](https://discourse.charmhub.io/), to be published in the  [Juju OLM Docs](https://juju.is/docs/olm). Short is great. 

- For wider reach, add your tutorial on tutorials.ubuntu.com. The process is really simple. All you need is a text file and a pull request on GitHub.
-->


<!-- LINKS -->


<!---
# General advice

## Strategies

- call juju again, e.g. `juju-ip`
- full custom command

Things to cover

- Python
  - use  `setuptools`
  - use the `entry_points` functionality to allow users to easily put things on their `$PATH`

Things to be aware of

- Users are not aware that they're calling a plugin 

Tools

- Python: libjuju
- Javascript: libjuju 

-->

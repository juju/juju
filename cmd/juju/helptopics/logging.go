// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package helptopics

const Logging = `
Juju has logging available for both client and server components. Most
users' exposure to the logging mechanism is through either the 'debug-log'
command, or through the log file stored on the bootstrap node at
/var/log/juju/all-machines.log.

All the agents have their own log files on the individual machines. So
for the bootstrap node, there is the machine agent log file at
/var/log/juju/machine-0.log.  When a unit is deployed on a machine,
a unit agent is started. This agent also logs to /var/log/juju and the
name of the log file is based on the id of the unit, so for wordpress/0
the log file is unit-wordpress-0.log.

Juju uses rsyslog to forward the content of all the log files on the machine
back to the bootstrap node, and they are accumulated into the all-machines.log
file.  Each line is prefixed with the source agent tag (also the same as
the filename without the extension).

Juju has a hierarchical logging system internally, and as a user you can
control how much information is logged out.

Output from the charm hook execution comes under the log name "unit".
By default Juju makes sure that this information is logged out at
the DEBUG level.  If you explicitly specify a value for unit, then
this is used instead.

Juju internal logging comes under the log name "juju".  Different areas
of the codebase have different anmes. For example:
  providers are under juju.provider
  workers are under juju.worker
  database parts are under juju.state

All the agents are started with all logging set to DEBUG. Which means you
see all the internal juju logging statements until the logging worker starts
and updates the logging configuration to be what is stored for the environment.

You can set the logging levels using a number of different mechanisms.

environments.yaml
 - all environments support 'logging-config' as a key
 - logging-config: ...
environment variable
 - export JUJU_LOGGING_CONFIG='...'
setting the logging-config at bootstrap time
 - juju bootstrap --logging-config='...'
juju set-environment logging-config='...'

Configuration values are separated by semicolons.

Examples:

  juju set-environment logging-config "juju=WARNING; unit=INFO"

Developers may well like:

  export JUJU_LOGGING_CONFIG='juju=INFO; juju.current.work.area=TRACE'

Valid logging levels:
  CRITICAL
  ERROR
  WARNING
  INFO
  DEBUG
  TRACE
`

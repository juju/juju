#!/bin/bash

# Executes the cmd in $1 and exits with an error if it fails.
execute ()
{
  CMD=$1
  MSG=$2
  echo -n $MSG.....
  ERR=$( { eval $CMD; } 2>&1 )
  if [ $? -ne 0 ]; then
    echo FAILED
    echo "------------------------------------------------------------"
    echo "Command failed: $CMD"
    echo "Error: $ERR"
    echo "------------------------------------------------------------"
    exit 1;
  fi
  echo SUCCESS
}

next_step()
{
  echo
  echo "**************************************************************"
  echo $1
  echo "**************************************************************"
}


echo
next_step "Performing Juju backup of critical files"
if [ -d juju-backup ]; then
  echo Older juju backup exists, moving to juju-backup-previous
  execute 'sudo -n rm -rf juju-backup-previous' "Removing existing backup archive"
  execute 'sudo -n mv juju-backup juju-backup-previous' "Archiving backup";
fi
execute 'mkdir juju-backup' "Making backup directory"
cd juju-backup

# Mongo requires that a locale is set
LC_ALL="en_US.UTF-8"
export LC_ALL

#---------------------------------------------------------------------
next_step "Backing up mongo database"
execute 'sudo -n stop juju-db' " Stopping mongo"
execute 'sudo -n mongodump --dbpath /var/lib/juju/db' "Backing up mongo"
execute 'sudo -n start juju-db' "Starting mongo"

#---------------------------------------------------------------------
next_step "Copying Juju configuration"
# upstart configuration files for juju-db, machine agent, unit agent(s)
execute 'mkdir upstart' "Making upstart backup directory"
execute 'cp /etc/init/juju*.conf upstart' "Copying upstart scripts"

# agent configuration directories in /var/lib/juju
# (includes the config, server.pem, tools)
execute 'mkdir juju_config' "Making juju_config backup directory"
execute 'sudo -n cp -r /var/lib/juju/agents juju_config' "Copying agent config"
execute 'sudo -n cp -r /var/lib/juju/tools juju_config' "Copying agent tools"
execute 'sudo -n cp /var/lib/juju/server.pem juju_config' "Copying server certificate"

# ~/.ssh/authorized_keys
execute 'mkdir ssh_keys' "Making ssh_keys backup directory"
execute 'cp  ~/.ssh/authorized_keys ssh_keys' "Copying ssh keys"

# /etc/rsyslog.d/*juju* config files for the agents
execute 'mkdir rsyslogd' "Making rsyslogd backup directory"
execute 'sudo -n cp /etc/rsyslog.d/*juju*.conf rsyslogd' "Copying rsyslog config"

# /var/log/juju/all-machines.log
execute 'mkdir logs' "Making logs backup directory"
execute 'sudo -n cp /var/log/juju/all-machines.log logs' "Copying log files"

#---------------------------------------------------------------------
next_step "Creating tarball"
execute 'sudo -n tar -zvcf juju-backup.tar.gz *' "Performing tar"

echo
echo
echo "Juju backup finished."
echo
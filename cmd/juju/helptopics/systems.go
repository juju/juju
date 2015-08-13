// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package helptopics

const JujuSystems = `

In order to use the multiple environment features of JES, you need to enable
a development feature flag:

    export JUJU_DEV_FEATURE_FLAGS=jes

This should be the default behaviour with Juju 1.26.

A Juju Environment System (JES), also sometimes shortened to 'juju system',
describes the environment that runs and manages the Juju API servers and the
underlying database.

This initial environment is also called the system environment, and is what is
created when the bootstrap command is called.  This system environment is a
normal Juju environment that just happens to have machines that manage Juju.

In order to keep a clean separation of concerns, it is considered best
practice to create additional environments for real workload deployment.

Services can still be deployed to the system environment, but it is generally
expected that these services are more for management and monitoring purposes,
like landscape and nagios.

When creating a Juju system that is going to be used by more than one person,
it is good practice to create users for each individual that will be accessing
the environments.

Users are managed within the Juju system using the 'juju user' command. This
allows the creation, listing, and disabling of users. When a juju system is
initially bootstrapped, there is only one user.  Additional users are created
as follows:

    $ juju user add bob "Bob Brown"
    user "Bob Brown (bob)" added
    server file written to /current/working/directory/bob.server

This command will create a local file "bob.server". The name of the file is
customisable using the --output option on the command. This 'server file'
should then be sent to Bob. Bob can then use this file to login to the Juju
system.

The system file contains everything that Juju needs to connect to the API
server of the Juju system. It has the network address, server certificate,
username and a randomly generated password.

Juju needs to have a name for the system when Bob calls the login command.
This is used to identify the system by name for other commands, like switch.

When Bob logs in to the system, a different random password is generated and
cached locally. This does mean that this particular server file is not usable
a second time. If Bob does not want to change the password, he can use the
--keep-password flag.

    $ juju system login --server=bob.server staging
    cached connection details as system "staging"
    -> staging (system)

Bob can then list all the environments within that system that he has access
to:

    $ juju system environments
    NAME  OWNER  LAST CONNECTION

The list could well be empty. Bob can create an environment to use:

    $ juju system create-environment test
    created environment "test"
    staging (system) -> test

When the environment has been created, it becomes the current environment. A
new environment has no machines, and no services.

    $ juju status
    environment: test
    machines: {}
    services: {}

Bob wants to collaborate with Mary on this environment. A user for Mary needs
to exist in the system before Bob is able to share the environment with her.

    $ juju environment share mary
    ERROR could not share environment: user "mary" does not exist locally: user "mary" not found

Bob gets the system administrator to add a user for Mary, and then shares the
environment with Mary.

    $ juju environment share mary
    $ juju environment users
    NAME        DATE CREATED    LAST CONNECTION
    bob@local   5 minutes ago   just now
    mary@local  57 seconds ago  never connected

When Mary has used her credentials to connect to the juju system, she can see
Bob's environment.

    $ juju system environments
    NAME  OWNER      LAST CONNECTION
    test  bob@local  never connected

Mary can use this environment.

    $ juju system use-environment test
    mary-server (system) -> bob-test

The local name for the environment is by default 'owner-name', so since this
environment is owned by 'bob@local' and called test, for mary the environment
is called 'bob-test'.

`

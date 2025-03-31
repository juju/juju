(plugin-juju-metadata)=
# Plugin `juju-metadata`

```{important}

If you've installed `juju` via snap, this plugin is probably already on your $PATH, so you can go ahead and use it as you would any other `juju` command. 

```

**Usage:** 

    juju metadata [flags] <command> ...

**Summary:**

tools for generating and validating image and tools metadata

**Flags:**

    --debug  (= false)
    
Equivalent to --show-log --logging-config=<root>=DEBUG

    --description  (= false)
    
Show short description of plugin, if any

    -h, --help  (= false)
    
Show help on a command or other topic.

    --log-file (= "")
    
Path to write log to

    --logging-config (= "")
    
Specify log levels for modules

    -q, --quiet  (= false)
    
Show no informational output

    --show-log  (= false)
    
If set, write the log file to stderr

    -v, --verbose  (= false)
    
Show more verbose output

**Details:**

Juju metadata is used to find the correct image and agent binaries when bootstrapping a
Juju model.

commands:

    add-image       - adds image metadata to model
    delete-image    - deletes image metadata from environment
    generate-agents - generate simplestreams agent metadata
    generate-image  - generate simplestreams image metadata
    generate-tools  - Alias for 'generate-agents'.
    help            - Show help on a command or other topic.
    list-images     - lists cloud image metadata used when choosing an image to start
    sign            - sign simplestreams metadata
    validate-agents - check that compressed tar archives (.tgz) for the Juju agent binaries are available
    validate-images - validate image metadata and ensure image(s) exist for a model
    validate-tools  - Alias for 'validate-agents'.
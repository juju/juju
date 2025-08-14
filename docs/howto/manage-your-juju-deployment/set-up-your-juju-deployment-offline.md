(take-your-deployment-offline)=
# Set up your Juju deployment - offline

<!--This doc is intended to supersede https://discourse.charmhub.io/t/how-to-work-offline/1072 and the docs linked there.

IMO the doc has roughly the correct skeleton, though we may want to revisit the list of external services and we may want to include suggestions for server and proxy software, as in the now archived https://discourse.charmhub.io/t/offline-mode-strategies/1071.

When all is said and done, though, I feel the perspective still needs to be that of the constructs Juju provides, namely, the model-config keys, as it is that that will dictate whether you should plan to set up a local mirror or a proxy or rather download the resources beforehand.

PS Noticed some of the environment variables don't match what's in the list of model config keys. Does the envvar have to have a particular name, or can it be anything and it is something just by convention? Either way, we need to clarify.

Details:

https://discourse.charmhub.io/t/how-to-configure-juju-for-offline-usage/1068
>> we've incorporated the list of external sites and even added to it, but left out the detail about client-controller-machine and just linked to our ref docs on the bootstrap and deploy process -- though when you compare the list and those docs you realize those docs are missing some detail (cloud-images..., archive-..., and security-..., and container image registry)
>> we've incorporated and cleaned up the examples

https://discourse.charmhub.io/t/offline-mode-strategies/1071
>> This doc mentions a bunch of proxies and local mirrors that should be set, including suggestions for possible proxy software, and then the model-config keys that can be used to configure Juju to use those proxies / local mirrors.  The content duplicates some of the content in https://discourse.charmhub.io/t/how-to-configure-juju-for-offline-usage/1068  -- we've already incorporated all of that. However, we haven't yet incorporated the suggestions for server and proxy software.


https://discourse.charmhub.io/t/how-to-deploy-charms-offline/1069
>> This doc is all wrong. The current process would be to download the charms on a machine connected to the internet; move them to an offline machine; deploy locally. There is no mention of this here at all (as we don't support either proxies or mirrors?).

https://discourse.charmhub.io/t/how-to-install-snaps-offline/1179
>> This doc merely illustrates how to use the http-proxy model-config. We now also have more specific snap store proxy keys.

https://discourse.charmhub.io/t/how-to-use-the-localhost-cloud-offline/1070
>> This doc is merely featuring how to use the no-proxy key to exclude the localhost cloud from the list of things that you want to use a proxy.

-->

For an offline (to be more precise, proxy-restricted) deployment:

1. Set up a private cloud.

> See more: {ref}`List of supported clouds <list-of-supported-clouds>`

2. Figure out the list of external services required for your deployment and set up proxies / local mirrors for them. Depending on whether your deployment is on machines or Kubernetes, and on a localhost cloud or not, and which one, these services may include:

    - [https://streams.canonical.com](https://streams.canonical.com/) for agent binaries and LXD container and VM images;
    - [https://charmhub.io/](https://charmhub.io/) for charms, including the Juju controller charm;
    - [https://snapcraft.io/store](https://snapcraft.io/store) for Juju's internal database;
    - [http://cloud-images.ubuntu.com](http://cloud-images.ubuntu.com/) for base Ubuntu cloud machine images, and [http://archive.ubuntu.com](http://archive.ubuntu.com/) and [http://security.ubuntu.com](http://security.ubuntu.com/) for machine image upgrades;
    - a container image registry:
        - [https://hub.docker.com/](https://hub.docker.com/)
        - [https://gallery.ecr.aws/juju](https://gallery.ecr.aws/juju) (in Juju provide it as "public.ecr.aws")
        - [https://ghcr.io/juju](https://ghcr.io/juju)


3. Configure Juju to make use of the proxies / local mirrors you've set up by means of the following model configuration keys:

- {ref}`model-config-agent-metadata-url`
- {ref}`model-config-apt-ftp-proxy`
- {ref}`model-config-apt-http-proxy`
- {ref}`model-config-apt-https-proxy`
- {ref}`model-config-apt-mirror`
- {ref}`model-config-apt-no-proxy`
- {ref}`model-config-container-image-metadata-url`
- {ref}`model-config-ftp-proxy`
- {ref}`model-config-http-proxy`
- {ref}`model-config-https-proxy`
- {ref}`model-config-image-metadata-url`
- {ref}`model-config-juju-ftp-proxy`
- {ref}`model-config-juju-http-proxy`
- {ref}`model-config-juju-https-proxy`
- {ref}`model-config-juju-no-proxy`
- {ref}`model-config-no-proxy`
- {ref}`model-config-snap-http-proxy`
- {ref}`model-config-snap-https-proxy`
- {ref}`model-config-snap-store-assertions`
- {ref}`model-config-snap-store-proxy`
- {ref}`model-config-snap-store-proxy-url`


````{dropdown} Example: Configure the client to use an HTTP proxy


Set up an HTTP proxy, export it to an environment variable, then use the `http-proxy` model configuration key to point the client to that value.

<!--
``` text
export http_proxy=$PROXY_HTTP
```
-->

````

````{dropdown} Example: Configure all models to use an APT mirror


Set up an APT mirror, export it to the environment variable $MIRROR_APT, then set the `apt-mirror` model config key to point to that environment variable. For example, for a controller on AWS:

``` text
juju bootstrap --model-default apt-mirror=$MIRROR_APT aws
```

````

````{dropdown} Example: Have all models use local resources for both Juju agent binaries and cloud images


Get the resources for Juju agent binaries and cloud images locally; create simplestreams for these binaries and images (`juju metadata`); define and export export environment variables pointing to the simplestreams; then set the `agent-metadata-url` and `image-metadata-url` model configuration keys to point to those environment variables. For example:

``` text
juju bootstrap \
    --model-default agent-metadata-url=$LOCAL_AGENTS \
    --model-default image-metadata-url=$LOCAL_IMAGES \
    localhost
```

````


````{dropdown} Example: Set up HTTP and HTTPS proxies but exclude the localhost cloud


Set up HTTP and HTTPS proxies and define and export environment variables pointing to them (below, `PROXY_HTTP` and `PROXY_HTTPS`); define and export a variable pointing to the IP addresses for your `localhost` cloud to the environment variable (below,`PROXY_NO`); then bootstrap setting the `http_proxy`, `https_proxy`, and `no-proxy` model configuration keys to the corresponding environment variable. For example:

```text
$ export PROXY_HTTP=http://squid.internal:3128
$ export PROXY_HTTPS=http://squid.internal:3128
$ export PROXY_NO=$(echo localhost 127.0.0.1 10.245.67.130 10.44.139.{1..255} | sed 's/ /,/g')

$ export http_proxy=$PROXY_HTTP
$ export https_proxy=$PROXY_HTTP
$ export no_proxy=$PROXY_NO

$ juju bootstrap \
--model-default http-proxy=$PROXY_HTTP \
--model-default https-proxy=$PROXY_HTTPS \
--model-default no-proxy=$PROXY_NO \
localhost lxd
```

````


4. Continue as usual by setting up users, storage, etc.; adding models; and deploying, configuring, integrating, etc., applications.

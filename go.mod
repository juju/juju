module github.com/juju/juju

go 1.14

require (
	github.com/Azure/azure-sdk-for-go v42.0.0+incompatible
	github.com/Azure/go-autorest/autorest v0.10.0
	github.com/Azure/go-autorest/autorest/adal v0.8.3
	github.com/Azure/go-autorest/autorest/date v0.2.0
	github.com/Azure/go-autorest/autorest/mocks v0.3.0
	github.com/Azure/go-autorest/autorest/to v0.3.0
	github.com/EvilSuperstars/go-cidrman v0.0.0-20170211231153-4e5a4a63d9b7
	github.com/altoros/gosigma v0.0.0-20200420012028-063911838a9e
	github.com/armon/go-metrics v0.0.0-20180917152333-f0300d1749da
	github.com/aws/aws-sdk-go v1.29.8
	github.com/bmizerany/pat v0.0.0-20160217103242-c068ca2f0aac
	github.com/chzyer/readline v0.0.0-20180603132655-2972be24d48e
	github.com/coreos/go-systemd/v22 v22.0.0-20200316104309-cb8b64719ae3
	github.com/docker/distribution v2.6.0-rc.1.0.20180522175653-f0cc92778478+incompatible
	github.com/dustin/go-humanize v1.0.0
	github.com/go-macaroon-bakery/macaroon-bakery/v3 v3.0.0-20210309064400-d73aa8f92aa2
	github.com/golang/mock v1.4.3
	github.com/google/go-querystring v0.0.0-20160401233042-9235644dd9e5
	github.com/googleapis/gnostic v0.2.0
	github.com/gorilla/handlers v0.0.0-20170224193955-13d73096a474
	github.com/gorilla/schema v0.0.0-20160426231512-08023a0215e7
	github.com/gorilla/websocket v1.4.2
	github.com/gosuri/uitable v0.0.1
	github.com/hashicorp/go-msgpack v0.5.5
	github.com/hashicorp/raft v2.0.0-20200420012049-88ad3b3f0a54+incompatible
	github.com/hashicorp/raft-boltdb v0.0.0-20171010151810-6e5ba93211ea
	github.com/joyent/gocommon v0.0.0-20160320193133-ade826b8b54e
	github.com/joyent/gosdc v0.0.0-20140524000815-2f11feadd2d9
	github.com/joyent/gosign v0.0.0-20140524000734-0da0d5f13420
	github.com/juju/ansiterm v0.0.0-20180109212912-720a0952cc2a
	github.com/juju/blobstore/v2 v2.0.0-20210302045357-edd2b24570b7
	github.com/juju/bundlechanges v1.0.1-0.20210209114844-acd700ed48fe
	github.com/juju/charm/v7 v7.0.4-0.20210312103909-46ac1f0b2db4
	github.com/juju/charmrepo/v5 v5.0.0-20210309073708-d0dc3a75f085
	github.com/juju/clock v0.0.0-20190205081909-9c5c9712527c
	github.com/juju/cmd v0.0.0-20200108104440-8e43f3faa5c9
	github.com/juju/collections v0.0.0-20200605021417-0d0ec82b7271
	github.com/juju/description/v2 v2.0.0-20200623093622-bd6ec044a72a
	github.com/juju/errors v0.0.0-20200330140219-3fe23663418f
	github.com/juju/featureflag v0.0.0-20200423045028-e2f9e1cb1611
	github.com/juju/gnuflag v0.0.0-20171113085948-2ce1bb71843d
	github.com/juju/go-oracle-cloud v0.0.0-20170421134547-932a8cea00a1
	github.com/juju/gojsonschema v0.0.0-20150312170016-e1ad140384f2
	github.com/juju/gomaasapi v0.0.0-20190826212825-0ab1eb636aba
	github.com/juju/idmclient/v2 v2.0.0-20210309081103-6b4a5212f851
	github.com/juju/jsonschema v0.0.0-20161102181919-a0ef8b74ebcf
	github.com/juju/jsonschema-gen v0.0.0-20200416014454-d924343d72b2
	github.com/juju/loggo v0.0.0-20200526014432-9ce3a2e09b5e
	github.com/juju/mgo/v2 v2.0.0-20210302023703-70d5d206e208
	github.com/juju/mutex v0.0.0-20180619145857-d21b13acf4bf
	github.com/juju/names/v4 v4.0.0-20200923012352-008effd8611b
	github.com/juju/naturalsort v0.0.0-20180423034842-5b81707e882b
	github.com/juju/os v1.1.1
	github.com/juju/packaging v0.0.0-20210322161715-32d1b6c12454
	github.com/juju/persistent-cookiejar v0.0.0-20170428161559-d67418f14c93
	github.com/juju/proxy v0.0.0-20180523025733-5f8741c297b4
	github.com/juju/pubsub v0.0.0-20190419131051-c1f7536b9cc6
	github.com/juju/ratelimit v1.0.2-0.20191002062651-f60b32039441
	github.com/juju/replicaset v0.0.0-20210302050932-0303c8575745
	github.com/juju/retry v0.0.0-20180821225755-9058e192b216
	github.com/juju/rfc v0.0.0-20180510112117-b058ad085c94
	github.com/juju/romulus v0.0.0-20210309074704-4fa3bbd32568
	github.com/juju/rpcreflect v0.0.0-20200416001309-bb46e9ba1476
	github.com/juju/schema v1.0.1-0.20190814234152-1f8aaeef0989
	github.com/juju/terms-client/v2 v2.0.0-20210309081804-aed8368405f6
	github.com/juju/testing v0.0.0-20210302031854-2c7ee8570c07
	github.com/juju/txn v0.0.0-20210302043154-251cea9e140a
	github.com/juju/utils v0.0.0-20200604140309-9d78121a29e0
	github.com/juju/version v0.0.0-20210303051006-2015802527a8
	github.com/juju/webbrowser v0.0.0-20180907093207-efb9432b2bcb
	github.com/juju/worker/v2 v2.0.0-20200916234526-d6e694f1c54a
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51
	github.com/kr/pretty v0.2.1
	github.com/lxc/lxd v0.0.0-20210308173700-befe9fce0a1f
	github.com/mattn/go-isatty v0.0.4
	github.com/oracle/oci-go-sdk v5.7.0+incompatible
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.5.1
	github.com/prometheus/client_model v0.2.0
	github.com/vmware/govmomi v0.21.1-0.20191008161538-40aebf13ba45
	golang.org/x/crypto v0.0.0-20210220033148-5ea612d1eb83
	golang.org/x/net v0.0.0-20210226172049-e18ecbb05110
	golang.org/x/oauth2 v0.0.0-20200107190931-bf48bf16ab8d
	golang.org/x/sys v0.0.0-20210309074719-68d13333faf2
	golang.org/x/tools v0.0.0-20190920225731-5eefd052ad72
	google.golang.org/api v0.4.0
	gopkg.in/amz.v3 v3.0.0-20200811022415-7b63e5e39741
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c
	gopkg.in/goose.v2 v2.0.1
	gopkg.in/httprequest.v1 v1.2.0
	gopkg.in/ini.v1 v1.10.1
	gopkg.in/juju/environschema.v1 v1.0.0
	gopkg.in/juju/names.v3 v3.0.0-20200424031302-5630f3ed762f // indirect
	gopkg.in/macaroon.v2 v2.1.0
	gopkg.in/natefinch/lumberjack.v2 v2.0.0
	gopkg.in/natefinch/npipe.v2 v2.0.0-20160621034901-c1b8fa8bdcce
	gopkg.in/retry.v1 v1.0.3
	gopkg.in/tomb.v2 v2.0.0-20161208151619-d5d1b5820637
	gopkg.in/yaml.v2 v2.4.0
	k8s.io/api v0.0.0-20200131193051-d9adff57e763
	k8s.io/apiextensions-apiserver v0.0.0-20200131201446-6910daba737d
	k8s.io/apimachinery v0.17.5-beta.0
	k8s.io/client-go v0.0.0-20200131194156-19522ff28802
)

replace github.com/altoros/gosigma => github.com/juju/gosigma v0.0.0-20200420012028-063911838a9e

replace gopkg.in/natefinch/lumberjack.v2 => github.com/juju/lumberjack v2.0.0-20200420012306-ddfd864a6ade+incompatible

replace github.com/hashicorp/raft => github.com/juju/raft v2.0.0-20200420012049-88ad3b3f0a54+incompatible

replace gopkg.in/yaml.v2 => github.com/juju/yaml v0.0.0-20200420012109-12a32b78de07

replace github.com/dustin/go-humanize v1.0.0 => github.com/dustin/go-humanize v0.0.0-20141228071148-145fabdb1ab7

replace github.com/hashicorp/raft-boltdb => github.com/juju/raft-boltdb v0.0.0-20200518034108-40b112c917c5

replace gopkg.in/juju/names.v3 => github.com/juju/names v0.0.0-20200331100531-2c9a102df211

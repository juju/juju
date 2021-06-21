module github.com/juju/juju

go 1.16

require (
	github.com/Azure/azure-sdk-for-go v46.4.0+incompatible
	github.com/Azure/go-autorest/autorest v0.11.6
	github.com/Azure/go-autorest/autorest/adal v0.9.4
	github.com/Azure/go-autorest/autorest/date v0.3.0
	github.com/Azure/go-autorest/autorest/mocks v0.4.1
	github.com/Azure/go-autorest/autorest/to v0.3.0
	github.com/Azure/go-autorest/autorest/validation v0.3.1 // indirect
	github.com/EvilSuperstars/go-cidrman v0.0.0-20170211231153-4e5a4a63d9b7
	github.com/altoros/gosigma v0.0.0-20150408145232-31228935eec6
	github.com/armon/go-metrics v0.0.0-20180917152333-f0300d1749da
	github.com/aws/aws-sdk-go v1.36.2
	github.com/bmizerany/pat v0.0.0-20160217103242-c068ca2f0aac
	github.com/canonical/pebble v0.0.0-20210609205628-b152ff448bbe
	github.com/chzyer/readline v0.0.0-20180603132655-2972be24d48e
	github.com/coreos/go-systemd/v22 v22.0.0-20200316104309-cb8b64719ae3
	github.com/dnaeon/go-vcr v1.1.0 // indirect
	github.com/docker/distribution v2.7.1+incompatible
	github.com/dustin/go-humanize v1.0.0
	github.com/flosch/pongo2 v0.0.0-20200913210552-0d938eb266f3 // indirect
	github.com/go-goose/goose/v3 v3.0.0-20210609085303-5be176eecc0f
	github.com/go-logr/logr v0.2.0
	github.com/go-macaroon-bakery/macaroon-bakery/v3 v3.0.0-20210309064400-d73aa8f92aa2
	github.com/golang/mock v1.5.0
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/google/go-querystring v1.0.0
	github.com/google/uuid v1.2.0 // indirect
	github.com/googleapis/gnostic v0.4.1
	github.com/gorilla/handlers v0.0.0-20170224193955-13d73096a474
	github.com/gorilla/schema v0.0.0-20160426231512-08023a0215e7
	github.com/gorilla/websocket v1.4.2
	github.com/gosuri/uitable v0.0.1
	github.com/hashicorp/go-immutable-radix v1.3.0 // indirect
	github.com/hashicorp/go-msgpack v0.5.5
	github.com/hashicorp/go-uuid v1.0.1 // indirect
	github.com/hashicorp/golang-lru v0.5.4 // indirect
	github.com/hashicorp/raft v2.0.0-20200420012049-88ad3b3f0a54+incompatible
	github.com/hashicorp/raft-boltdb v0.0.0-20171010151810-6e5ba93211ea
	github.com/imdario/mergo v0.3.10 // indirect
	github.com/juju/ansiterm v0.0.0-20180109212912-720a0952cc2a
	github.com/juju/blobstore/v2 v2.0.0-20210302045357-edd2b24570b7
	github.com/juju/charm/v9 v9.0.0-20210512004933-c21e01ffd4ad
	github.com/juju/charmrepo/v7 v7.0.0-20210427073450-742f84f1c0ff
	github.com/juju/clock v0.0.0-20190205081909-9c5c9712527c
	github.com/juju/cmd v0.0.0-20200108104440-8e43f3faa5c9
	github.com/juju/collections v0.0.0-20200605021417-0d0ec82b7271
	github.com/juju/description/v3 v3.0.0-20210507010228-732e570aa3b2
	github.com/juju/errors v0.0.0-20200330140219-3fe23663418f
	github.com/juju/featureflag v0.0.0-20200423045028-e2f9e1cb1611
	github.com/juju/gnuflag v0.0.0-20171113085948-2ce1bb71843d
	github.com/juju/gojsonschema v0.0.0-20150312170016-e1ad140384f2
	github.com/juju/gomaasapi/v2 v2.0.0-20210323144809-92beddd020fe
	github.com/juju/http/v2 v2.0.0-20210616081525-ede00d07798a
	github.com/juju/idmclient/v2 v2.0.0-20210309081103-6b4a5212f851
	github.com/juju/jsonschema v0.0.0-20210422141032-b0ff291abe9c
	github.com/juju/jsonschema-gen v0.0.0-20200416014454-d924343d72b2
	github.com/juju/loggo v0.0.0-20200526014432-9ce3a2e09b5e
	github.com/juju/mgo/v2 v2.0.0-20210414025616-e854c672032f
	github.com/juju/mutex v0.0.0-20180619145857-d21b13acf4bf
	github.com/juju/names/v4 v4.0.0-20200929085019-be23e191fee0
	github.com/juju/naturalsort v0.0.0-20180423034842-5b81707e882b
	github.com/juju/os/v2 v2.1.2
	github.com/juju/packaging v0.0.0-20210602012220-a1d8f0c5acd9
	github.com/juju/persistent-cookiejar v0.0.0-20170428161559-d67418f14c93
	github.com/juju/proxy v0.0.0-20180523025733-5f8741c297b4
	github.com/juju/pubsub v0.0.0-20190419131051-c1f7536b9cc6
	github.com/juju/ratelimit v1.0.2-0.20191002062651-f60b32039441
	github.com/juju/replicaset/v2 v2.0.0-20210310024806-bbbdc5f31eb3
	github.com/juju/retry v0.0.0-20180821225755-9058e192b216
	github.com/juju/rfc/v2 v2.0.0-20210319034215-ed820200fad3
	github.com/juju/romulus v0.0.0-20210309074704-4fa3bbd32568
	github.com/juju/rpcreflect v0.0.0-20200416001309-bb46e9ba1476
	github.com/juju/schema v1.0.1-0.20190814234152-1f8aaeef0989
	github.com/juju/terms-client/v2 v2.0.0-20210422053140-27f71c100676
	github.com/juju/testing v0.0.0-20210324180055-18c50b0c2098
	github.com/juju/txn/v2 v2.0.0-20210407000251-11166e89894c
	github.com/juju/utils v0.0.0-20200604140309-9d78121a29e0
	github.com/juju/utils/v2 v2.0.0-20210305225158-eedbe7b6b3e2
	github.com/juju/version/v2 v2.0.0-20210319015800-dcfac8f4f057
	github.com/juju/webbrowser v1.0.0
	github.com/juju/worker/v2 v2.0.0-20200916234526-d6e694f1c54a
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51
	github.com/kr/pretty v0.2.1
	github.com/lxc/lxd v0.0.0-20210607205159-a7c206b5233d
	github.com/mattn/go-colorable v0.1.8 // indirect
	github.com/mattn/go-isatty v0.0.12
	github.com/mitchellh/go-linereader v0.0.0-20190213213312-1b945b3263eb
	github.com/onsi/ginkgo v1.14.2 // indirect
	github.com/onsi/gomega v1.10.4 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/oracle/oci-go-sdk v5.7.0+incompatible
	github.com/pascaldekloe/goe v0.1.0 // indirect
	github.com/pborman/uuid v1.2.1 // indirect
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.7.1
	github.com/prometheus/client_model v0.2.0
	github.com/rogpeppe/fastuuid v1.2.0 // indirect
	github.com/satori/go.uuid v1.2.0
	github.com/smartystreets/goconvey v1.6.4 // indirect
	github.com/vishvananda/netlink v1.1.0
	github.com/vmware/govmomi v0.21.1-0.20191008161538-40aebf13ba45
	golang.org/x/crypto v0.0.0-20210513164829-c07d793c2f9a
	golang.org/x/mod v0.4.0 // indirect
	golang.org/x/net v0.0.0-20210525063256-abc453219eb5
	golang.org/x/oauth2 v0.0.0-20200107190931-bf48bf16ab8d
	golang.org/x/sync v0.0.0-20201020160332-67f06af15bc9
	golang.org/x/sys v0.0.0-20210608053332-aa57babbf139
	golang.org/x/time v0.0.0-20200630173020-3af7569d3a1e // indirect
	golang.org/x/tools v0.0.0-20210105210202-9ed45478a130
	google.golang.org/api v0.29.0
	google.golang.org/appengine v1.6.6 // indirect
	google.golang.org/genproto v0.0.0-20200726014623-da3ae01ef02d // indirect
	google.golang.org/grpc v1.33.1 // indirect
	gopkg.in/amz.v3 v3.0.0-20201001071545-24fc1eceb27b
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c
	gopkg.in/httprequest.v1 v1.2.1
	gopkg.in/ini.v1 v1.51.0
	gopkg.in/juju/environschema.v1 v1.0.1-0.20201027142642-c89a4490670a
	gopkg.in/macaroon.v2 v2.1.0
	gopkg.in/natefinch/lumberjack.v2 v2.0.0
	gopkg.in/natefinch/npipe.v2 v2.0.0-20160621034901-c1b8fa8bdcce
	gopkg.in/retry.v1 v1.0.3
	gopkg.in/robfig/cron.v2 v2.0.0-20150107220207-be2e0b0deed5 // indirect
	gopkg.in/tomb.v2 v2.0.0-20161208151619-d5d1b5820637
	gopkg.in/yaml.v2 v2.4.0
	gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b
	k8s.io/api v0.19.6
	k8s.io/apiextensions-apiserver v0.19.6
	k8s.io/apimachinery v0.19.6
	k8s.io/client-go v0.19.6
	k8s.io/klog/v2 v2.3.0
)

replace github.com/altoros/gosigma => github.com/juju/gosigma v0.0.0-20200420012028-063911838a9e

replace gopkg.in/natefinch/lumberjack.v2 => github.com/juju/lumberjack v2.0.0-20200420012306-ddfd864a6ade+incompatible

replace github.com/hashicorp/raft => github.com/juju/raft v2.0.0-20200420012049-88ad3b3f0a54+incompatible

replace gopkg.in/yaml.v2 => github.com/juju/yaml v0.0.0-20200420012109-12a32b78de07

replace github.com/dustin/go-humanize v1.0.0 => github.com/dustin/go-humanize v0.0.0-20141228071148-145fabdb1ab7

replace github.com/hashicorp/raft-boltdb => github.com/juju/raft-boltdb v0.0.0-20200518034108-40b112c917c5

replace (
	k8s.io/api v0.0.0 => k8s.io/api v0.19.6
	k8s.io/apiextensions-apiserver v0.0.0 => k8s.io/apiextensions-apiserver v0.19.6
	k8s.io/apimachinery v0.0.0 => k8s.io/apimachinery v0.19.6
	k8s.io/client-go v0.0.0 => k8s.io/client-go v0.19.6
)

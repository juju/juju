module github.com/juju/juju

go 1.14

require (
	github.com/Azure/azure-sdk-for-go v46.4.0+incompatible
	github.com/Azure/go-autorest/autorest v0.11.6
	github.com/Azure/go-autorest/autorest/adal v0.9.4
	github.com/Azure/go-autorest/autorest/date v0.3.0
	github.com/Azure/go-autorest/autorest/mocks v0.4.1
	github.com/Azure/go-autorest/autorest/to v0.3.0
	github.com/Azure/go-autorest/autorest/validation v0.2.0 // indirect
	github.com/EvilSuperstars/go-cidrman v0.0.0-20170211231153-4e5a4a63d9b7
	github.com/altoros/gosigma v0.0.0-20200420012028-063911838a9e
	github.com/armon/go-metrics v0.0.0-20180917152333-f0300d1749da
	github.com/aws/aws-sdk-go v1.36.2
	github.com/bmizerany/pat v0.0.0-20160217103242-c068ca2f0aac
	github.com/chzyer/readline v0.0.0-20180603132655-2972be24d48e
	github.com/coreos/go-systemd/v22 v22.0.0-20200316104309-cb8b64719ae3
	github.com/dnaeon/go-vcr v1.0.1 // indirect
	github.com/docker/distribution v2.7.1+incompatible
	github.com/docker/spdystream v0.0.0-20181023171402-6480d4af844c // indirect
	github.com/dustin/go-humanize v1.0.0
	github.com/evanphx/json-patch v4.5.0+incompatible // indirect
	github.com/flosch/pongo2 v0.0.0-20141028000813-5e81b817a0c4 // indirect
	github.com/golang/mock v1.4.3
	github.com/google/go-cmp v0.5.1 // indirect
	github.com/google/go-querystring v1.0.0
	github.com/googleapis/gnostic v0.4.0
	github.com/gorilla/handlers v0.0.0-20170224193955-13d73096a474
	github.com/gorilla/schema v0.0.0-20160426231512-08023a0215e7
	github.com/gorilla/websocket v1.4.2
	github.com/gosuri/uitable v0.0.1
	github.com/hashicorp/go-immutable-radix v1.0.0 // indirect
	github.com/hashicorp/go-msgpack v0.5.5
	github.com/hashicorp/golang-lru v0.5.4 // indirect
	github.com/hashicorp/raft v2.0.0-20200420012049-88ad3b3f0a54+incompatible
	github.com/hashicorp/raft-boltdb v0.0.0-20171010151810-6e5ba93211ea
	github.com/hpidcock/juju-fake-init v0.0.0-20201026041434-e95018795575
	github.com/imdario/mergo v0.3.10 // indirect
	github.com/juju/ansiterm v0.0.0-20180109212912-720a0952cc2a
	github.com/juju/bundlechanges/v3 v3.0.0-20201118044951-0e6ce11ef6c6
	github.com/juju/charm/v8 v8.0.0-20201119121237-bd48f89b02b9
	github.com/juju/charmrepo/v6 v6.0.0-20201118043529-e9fbdc1a746f
	github.com/juju/clock v0.0.0-20190205081909-9c5c9712527c
	github.com/juju/cmd v0.0.0-20200108104440-8e43f3faa5c9
	github.com/juju/collections v0.0.0-20200605021417-0d0ec82b7271
	github.com/juju/description/v2 v2.0.0-20201013195630-fc4923d04798
	github.com/juju/errors v0.0.0-20200330140219-3fe23663418f
	github.com/juju/featureflag v0.0.0-20200423045028-e2f9e1cb1611
	github.com/juju/gnuflag v0.0.0-20171113085948-2ce1bb71843d
	github.com/juju/go4 v0.0.0-20160222163258-40d72ab9641a // indirect
	github.com/juju/gojsonschema v0.0.0-20150312170016-e1ad140384f2
	github.com/juju/gomaasapi v0.0.0-20190826212825-0ab1eb636aba
	github.com/juju/http v0.0.0-20201019013536-69ae1d429836
	github.com/juju/jsonschema v0.0.0-20161102181919-a0ef8b74ebcf
	github.com/juju/jsonschema-gen v0.0.0-20200416014454-d924343d72b2
	github.com/juju/loggo v0.0.0-20200526014432-9ce3a2e09b5e
	github.com/juju/lru v0.0.0-20190314140547-92a0afabdc41 // indirect
	github.com/juju/mutex v0.0.0-20180619145857-d21b13acf4bf
	github.com/juju/names/v4 v4.0.0-20200929085019-be23e191fee0
	github.com/juju/naturalsort v0.0.0-20180423034842-5b81707e882b
	github.com/juju/os/v2 v2.1.0
	github.com/juju/packaging v0.0.0-20200421095529-970596d2622a
	github.com/juju/persistent-cookiejar v0.0.0-20170428161559-d67418f14c93
	github.com/juju/proxy v0.0.0-20180523025733-5f8741c297b4
	github.com/juju/pubsub v0.0.0-20190419131051-c1f7536b9cc6
	github.com/juju/ratelimit v1.0.2-0.20191002062651-f60b32039441
	github.com/juju/replicaset v0.0.0-20190321104350-501ab59799b1
	github.com/juju/retry v0.0.0-20180821225755-9058e192b216
	github.com/juju/rfc v0.0.0-20180510112117-b058ad085c94
	github.com/juju/romulus v0.0.0-20191205211046-fd7cab26ac5f
	github.com/juju/rpcreflect v0.0.0-20200416001309-bb46e9ba1476
	github.com/juju/schema v1.0.1-0.20190814234152-1f8aaeef0989
	github.com/juju/systems v0.0.0-20200925032749-8c613192c759
	github.com/juju/terms-client v1.0.2-0.20200331164339-fab45ea044ae
	github.com/juju/testing v0.0.0-20201030020617-7189b3728523
	github.com/juju/txn v0.0.0-20201106143451-65a67d427a16
	github.com/juju/usso v0.0.0-20160401104424-68a59c96c178 // indirect
	github.com/juju/utils v0.0.0-20200604140309-9d78121a29e0
	github.com/juju/utils/v2 v2.0.0-20200923005554-4646bfea2ef1
	github.com/juju/version v0.0.0-20191219164919-81c1be00b9a6
	github.com/juju/webbrowser v1.0.0
	github.com/juju/worker/v2 v2.0.0-20200916234526-d6e694f1c54a
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51
	github.com/kr/pretty v0.2.1
	github.com/lestrrat/go-jspointer v0.0.0-20160229021354-f4881e611bdb // indirect
	github.com/lestrrat/go-jsref v0.0.0-20160601013240-e452c7b5801d // indirect
	github.com/lestrrat/go-jsschema v0.0.0-20160903131957-b09d7650b822 // indirect
	github.com/lestrrat/go-jsval v0.0.0-20161012045717-b1258a10419f // indirect
	github.com/lestrrat/go-pdebug v0.0.0-20160817063333-2e6eaaa5717f // indirect
	github.com/lestrrat/go-structinfo v0.0.0-20160308131105-f74c056fe41f // indirect
	github.com/lxc/lxd v0.0.0-20201127143816-0245f4a840c6
	github.com/mattn/go-isatty v0.0.12
	github.com/mitchellh/go-linereader v0.0.0-20190213213312-1b945b3263eb
	github.com/opencontainers/go-digest v1.0.0-rc1 // indirect
	github.com/oracle/oci-go-sdk v5.7.0+incompatible
	github.com/pascaldekloe/goe v0.1.0 // indirect
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.7.1
	github.com/prometheus/client_model v0.2.0
	github.com/rogpeppe/fastuuid v1.2.0 // indirect
	github.com/satori/go.uuid v1.2.0
	github.com/smartystreets/goconvey v1.6.4 // indirect
	github.com/vmware/govmomi v0.21.1-0.20191008161538-40aebf13ba45
	github.com/xeipuuv/gojsonschema v1.2.0 // indirect
	golang.org/x/crypto v0.0.0-20201117144127-c1f2f97bffc9
	golang.org/x/net v0.0.0-20201110031124-69a78807bb2b
	golang.org/x/oauth2 v0.0.0-20200107190931-bf48bf16ab8d
	golang.org/x/sync v0.0.0-20200625203802-6e8e738ad208
	golang.org/x/sys v0.0.0-20201117222635-ba5294a509c7
	golang.org/x/time v0.0.0-20200630173020-3af7569d3a1e // indirect
	golang.org/x/tools v0.0.0-20200725200936-102e7d357031
	google.golang.org/api v0.29.0
	google.golang.org/appengine v1.6.6 // indirect
	google.golang.org/genproto v0.0.0-20200726014623-da3ae01ef02d // indirect
	gopkg.in/amz.v3 v3.0.0-20201001071545-24fc1eceb27b
	gopkg.in/check.v1 v1.0.0-20200902074654-038fdea0a05b
	gopkg.in/goose.v2 v2.0.1
	gopkg.in/httprequest.v1 v1.2.1
	gopkg.in/ini.v1 v1.10.1
	gopkg.in/juju/blobstore.v2 v2.0.0-20160125023703-51fa6e26128d
	gopkg.in/juju/environschema.v1 v1.0.1-0.20201027142642-c89a4490670a
	gopkg.in/juju/idmclient.v1 v1.0.0-20180320161856-203d20774ce8
	gopkg.in/juju/names.v3 v3.0.0-20200331100531-2c9a102df211 // indirect
	gopkg.in/juju/worker.v1 v1.0.0-20191018043616-19a698a7150f // indirect
	gopkg.in/macaroon-bakery.v2 v2.2.0
	gopkg.in/macaroon.v2 v2.1.0
	gopkg.in/mgo.v2 v2.0.0-20190816093944-a6b53ec6cb22
	gopkg.in/natefinch/lumberjack.v2 v2.0.0
	gopkg.in/natefinch/npipe.v2 v2.0.0-20160621034901-c1b8fa8bdcce
	gopkg.in/retry.v1 v1.0.2
	gopkg.in/robfig/cron.v2 v2.0.0-20150107220207-be2e0b0deed5 // indirect
	gopkg.in/tomb.v2 v2.0.0-20161208151619-d5d1b5820637
	gopkg.in/yaml.v2 v2.3.0
	gopkg.in/yaml.v3 v3.0.0-20200615113413-eeeca48fe776 // indirect
	k8s.io/api v0.18.6
	k8s.io/apiextensions-apiserver v0.18.6
	k8s.io/apimachinery v0.18.6
	k8s.io/client-go v0.18.6
	k8s.io/klog/v2 v2.3.0 // indirect
	k8s.io/utils v0.0.0-20200724153422-f32512634ab7 // indirect
)

replace github.com/altoros/gosigma => github.com/juju/gosigma v0.0.0-20200420012028-063911838a9e

replace gopkg.in/natefinch/lumberjack.v2 => github.com/juju/lumberjack v2.0.0-20200420012306-ddfd864a6ade+incompatible

replace gopkg.in/mgo.v2 => github.com/juju/mgo v2.0.0-20201106044211-d9585b0b0d28+incompatible

replace github.com/hashicorp/raft => github.com/juju/raft v2.0.0-20200420012049-88ad3b3f0a54+incompatible

replace gopkg.in/yaml.v2 => github.com/juju/yaml v0.0.0-20200420012109-12a32b78de07

replace github.com/dustin/go-humanize v1.0.0 => github.com/dustin/go-humanize v0.0.0-20141228071148-145fabdb1ab7

replace github.com/hashicorp/raft-boltdb => github.com/juju/raft-boltdb v0.0.0-20200518034108-40b112c917c5

replace (
	k8s.io/api v0.0.0 => k8s.io/api v0.18.6
	k8s.io/apiextensions-apiserver v0.0.0 => k8s.io/apiextensions-apiserver v0.18.6
	k8s.io/apimachinery v0.0.0 => k8s.io/apimachinery v0.18.6
	k8s.io/client-go v0.0.0 => k8s.io/client-go v0.18.6
)

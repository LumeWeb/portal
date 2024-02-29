module git.lumeweb.com/LumeWeb/portal

go 1.21.6

require (
	git.lumeweb.com/LumeWeb/libs5-go v0.0.0-20240229163213-7bd9cf11ae65
	github.com/AfterShip/email-verifier v1.4.0
	github.com/aws/aws-sdk-go-v2 v1.25.1
	github.com/aws/aws-sdk-go-v2/config v1.27.2
	github.com/aws/aws-sdk-go-v2/credentials v1.17.2
	github.com/aws/aws-sdk-go-v2/service/s3 v1.50.3
	github.com/casbin/casbin/v2 v2.82.0
	github.com/ddo/rq v0.0.0-20190828174524-b3daa55fcaba
	github.com/dnslink-std/go v0.6.0
	github.com/docker/go-units v0.5.0
	github.com/getkin/kin-openapi v0.118.0
	github.com/go-co-op/gocron/v2 v2.2.4
	github.com/go-gorm/caches/v4 v4.0.0
	github.com/go-resty/resty/v2 v2.11.0
	github.com/golang-jwt/jwt/v5 v5.2.0
	github.com/google/uuid v1.6.0
	github.com/hashicorp/go-plugin v1.6.0
	github.com/julienschmidt/httprouter v1.3.0
	github.com/mitchellh/mapstructure v1.5.0
	github.com/pquerna/otp v1.4.0
	github.com/redis/go-redis/v9 v9.5.1
	github.com/rs/cors v1.10.1
	github.com/samber/lo v1.39.0
	github.com/spf13/viper v1.18.2
	github.com/tus/tusd/v2 v2.2.3-0.20240125123123-9080d351525d
	github.com/vmihailenco/msgpack/v5 v5.4.1
	github.com/wneessen/go-mail v0.4.1
	go.etcd.io/bbolt v1.3.8
	go.sia.tech/core v0.1.12
	go.sia.tech/jape v0.11.1
	go.sia.tech/renterd v1.0.5
	go.uber.org/fx v1.20.1
	go.uber.org/zap v1.26.0
	go.uber.org/zap/exp v0.2.0
	golang.org/x/crypto v0.19.0
	google.golang.org/grpc v1.62.0
	google.golang.org/protobuf v1.32.0
	gorm.io/driver/mysql v1.5.4
	gorm.io/gorm v1.25.7
	lukechampine.com/blake3 v1.2.1
	nhooyr.io/websocket v1.8.10
)

require (
	github.com/aead/chacha20 v0.0.0-20180709150244-8b13a72661da // indirect
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.6.1 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.15.1 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.3.1 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.6.1 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.8.0 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.3.1 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.11.1 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.3.1 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.11.1 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.17.1 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.19.2 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.22.2 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.27.2 // indirect
	github.com/aws/smithy-go v1.20.1 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/boombuler/barcode v1.0.1-0.20190219062509-6c824513bacc // indirect
	github.com/casbin/govaluate v1.1.0 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/dchest/threefish v0.0.0-20120919164726-3ecf4c494abf // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/emirpasic/gods v1.18.1 // indirect
	github.com/fatih/color v1.14.1 // indirect
	github.com/fsnotify/fsnotify v1.7.0 // indirect
	github.com/go-openapi/jsonpointer v0.20.2 // indirect
	github.com/go-openapi/swag v0.22.8 // indirect
	github.com/go-sql-driver/mysql v1.7.1 // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/gorilla/websocket v1.5.1 // indirect
	github.com/hashicorp/go-hclog v1.6.2 // indirect
	github.com/hashicorp/hcl v1.0.0 // indirect
	github.com/hashicorp/yamux v0.1.1 // indirect
	github.com/hbollon/go-edlib v1.6.0 // indirect
	github.com/invopop/yaml v0.2.0 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	github.com/jonboulle/clockwork v0.4.0 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/klauspost/cpuid/v2 v2.2.6 // indirect
	github.com/klauspost/reedsolomon v1.12.1 // indirect
	github.com/magiconair/properties v1.8.7 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.19 // indirect
	github.com/matttproud/golang_protobuf_extensions/v2 v2.0.0 // indirect
	github.com/miekg/dns v1.1.58 // indirect
	github.com/mitchellh/go-testing-interface v0.0.0-20171004221916-a61a99592b77 // indirect
	github.com/mohae/deepcopy v0.0.0-20170929034955-c48cc78d4826 // indirect
	github.com/mr-tron/base58 v1.1.0 // indirect
	github.com/multiformats/go-base32 v0.0.3 // indirect
	github.com/multiformats/go-base36 v0.1.0 // indirect
	github.com/multiformats/go-multibase v0.2.0 // indirect
	github.com/oklog/run v1.0.0 // indirect
	github.com/olebedev/emitter v0.0.0-20230411050614-349169dec2ba // indirect
	github.com/pelletier/go-toml/v2 v2.1.0 // indirect
	github.com/perimeterx/marshmallow v1.1.5 // indirect
	github.com/prometheus/client_golang v1.18.0 // indirect
	github.com/prometheus/client_model v0.5.0 // indirect
	github.com/prometheus/common v0.45.0 // indirect
	github.com/prometheus/procfs v0.12.0 // indirect
	github.com/robfig/cron/v3 v3.0.1 // indirect
	github.com/sagikazarmark/locafero v0.4.0 // indirect
	github.com/sagikazarmark/slog-shim v0.1.0 // indirect
	github.com/sourcegraph/conc v0.3.0 // indirect
	github.com/spf13/afero v1.11.0 // indirect
	github.com/spf13/cast v1.6.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/subosito/gotenv v1.6.0 // indirect
	github.com/vmihailenco/tagparser/v2 v2.0.0 // indirect
	gitlab.com/NebulousLabs/bolt v1.4.4 // indirect
	gitlab.com/NebulousLabs/encoding v0.0.0-20200604091946-456c3dc907fe // indirect
	gitlab.com/NebulousLabs/entropy-mnemonics v0.0.0-20181018051301-7532f67e3500 // indirect
	gitlab.com/NebulousLabs/errors v0.0.0-20200929122200-06c536cf6975 // indirect
	gitlab.com/NebulousLabs/fastrand v0.0.0-20181126182046-603482d69e40 // indirect
	gitlab.com/NebulousLabs/go-upnp v0.0.0-20211002182029-11da932010b6 // indirect
	gitlab.com/NebulousLabs/log v0.0.0-20210609172545-77f6775350e2 // indirect
	gitlab.com/NebulousLabs/merkletree v0.0.0-20200118113624-07fbf710afc4 // indirect
	gitlab.com/NebulousLabs/persist v0.0.0-20200605115618-007e5e23d877 // indirect
	gitlab.com/NebulousLabs/ratelimit v0.0.0-20200811080431-99b8f0768b2e // indirect
	gitlab.com/NebulousLabs/siamux v0.0.2-0.20220630142132-142a1443a259 // indirect
	gitlab.com/NebulousLabs/threadgroup v0.0.0-20200608151952-38921fbef213 // indirect
	go.sia.tech/mux v1.2.0 // indirect
	go.sia.tech/siad v1.5.10-0.20230228235644-3059c0b930ca // indirect
	go.uber.org/dig v1.17.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/exp v0.0.0-20231219180239-dc181d75b848 // indirect
	golang.org/x/mod v0.15.0 // indirect
	golang.org/x/net v0.21.0 // indirect
	golang.org/x/sys v0.17.0 // indirect
	golang.org/x/text v0.14.0 // indirect
	golang.org/x/tools v0.18.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240123012728-ef4313101c80 // indirect
	gopkg.in/ini.v1 v1.67.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	lukechampine.com/frand v1.4.2 // indirect
)

replace (
	github.com/tus/tusd/v2 => github.com/LumeWeb/tusd/v2 v2.2.3-0.20240224143554-96925dd43120
	go.sia.tech/jape => github.com/LumeWeb/jape v0.0.0-20240204004049-ed792e7631cd
)

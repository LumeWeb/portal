module go.lumeweb.com/portal // v0.5.0

go 1.26.0

require (
	github.com/AfterShip/email-verifier v1.4.1
	github.com/DATA-DOG/go-sqlmock v1.5.2
	github.com/Oudwins/zog v0.22.2
	github.com/adjust/rmq/v5 v5.2.0
	github.com/aws/aws-sdk-go-v2 v1.42.1
	github.com/aws/aws-sdk-go-v2/config v1.32.29
	github.com/aws/aws-sdk-go-v2/credentials v1.19.28
	github.com/aws/aws-sdk-go-v2/service/s3 v1.105.0
	github.com/aws/smithy-go v1.27.3
	github.com/casbin/casbin/v3 v3.10.0
	github.com/casbin/gorm-adapter/v3 v3.41.0
	github.com/docker/go-units v0.5.0
	github.com/eventials/go-tus v0.0.0-20250612203642-7827b129cd4c
	github.com/fatih/structs v1.1.0
	github.com/gabriel-vasile/mimetype v1.4.13
	github.com/gammazero/workerpool v1.2.1
	github.com/go-co-op/gocron/mocks/v2 v2.0.0-20260709183641-10cdd5e5558f
	github.com/go-co-op/gocron/v2 v2.21.2
	github.com/go-gorm/caches/v4 v4.0.5
	github.com/go-kiss/monkey v0.0.0-20240508030602-d03795257a64
	github.com/go-sql-driver/mysql v1.10.0
	github.com/go-viper/mapstructure/v2 v2.5.0
	github.com/google/uuid v1.6.0
	github.com/invopop/jsonschema v0.14.0
	github.com/ipfs/go-cid v0.6.2
	github.com/johannesboyne/gofakes3 v0.0.0-20260208201424-4c385a1f6a73
	github.com/kenshaw/snaker v0.4.3
	github.com/knadh/koanf/parsers/json v1.0.0
	github.com/knadh/koanf/providers/confmap v1.0.0
	github.com/knadh/koanf/providers/rawbytes v1.0.0
	github.com/knadh/koanf/providers/structs v1.0.0
	github.com/knadh/koanf/v2 v2.3.5
	github.com/labstack/echo-contrib v0.17.4
	github.com/labstack/echo/v4 v4.15.4
	github.com/looplab/fsm v1.0.3
	github.com/mattn/go-sqlite3 v1.14.47
	github.com/miekg/dns v1.1.72
	github.com/multiformats/go-multibase v0.3.0
	github.com/multiformats/go-multihash v0.2.3
	github.com/pquerna/otp v1.5.0
	github.com/pressly/goose/v3 v3.27.2
	github.com/redis/go-redis/v9 v9.21.0
	github.com/samber/lo v1.53.0
	github.com/spf13/afero v1.15.0
	github.com/stretchr/testify v1.11.1
	github.com/tus/tusd/v2 v2.10.0
	github.com/urfave/cli/v3 v3.10.1
	github.com/wneessen/go-mail v0.8.1
	go.etcd.io/etcd/client/v3 v3.7.0
	go.lumeweb.com/configmanager v0.3.29
	go.lumeweb.com/event/v2 v2.1.0
	go.lumeweb.com/httputil v0.5.4
	go.lumeweb.com/portal-middleware v0.3.7
	go.lumeweb.com/portal-router v0.7.4
	go.opentelemetry.io/contrib/bridges/otelzap v0.19.0
	go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho v0.69.0
	go.opentelemetry.io/otel v1.44.0
	go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc v0.20.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.44.0
	go.opentelemetry.io/otel/log v0.20.0
	go.opentelemetry.io/otel/sdk v1.44.0
	go.opentelemetry.io/otel/sdk/log v0.20.0
	go.opentelemetry.io/otel/trace v1.44.0
	go.sia.tech/core v0.21.6
	go.sia.tech/coreutils v0.23.5
	go.sia.tech/renterd/v2 v2.9.2
	go.uber.org/mock v0.6.0
	go.uber.org/zap v1.28.0
	go.uber.org/zap/exp v0.3.0
	golang.org/x/crypto v0.54.0
	golang.org/x/exp v0.0.0-20260709172345-9ea1abe57597
	gopkg.in/yaml.v3 v3.0.1
	gorm.io/datatypes v1.2.7
	gorm.io/driver/mysql v1.6.0
	gorm.io/driver/sqlite v1.6.0
	gorm.io/gorm v1.31.2
	gorm.io/plugin/opentelemetry v0.1.16
	gorm.io/plugin/prometheus v0.1.0
)

require (
	github.com/alicebob/miniredis/v2 v2.37.0 // indirect
	github.com/bahlo/generic-list-go v0.2.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/bmatcuk/doublestar/v4 v4.9.1 // indirect
	github.com/buger/jsonparser v1.1.2 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/gammazero/deque v1.2.1 // indirect
	github.com/getkin/kin-openapi v0.134.0 // indirect
	github.com/ghodss/yaml v1.0.0 // indirect
	github.com/glebarez/go-sqlite v1.22.0 // indirect
	github.com/glebarez/sqlite v1.11.0 // indirect
	github.com/go-openapi/jsonpointer v0.22.5 // indirect
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/golang-sql/civil v0.0.0-20220223132316-b832511892a9 // indirect
	github.com/golang-sql/sqlexp v0.1.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.29.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/pgx/v5 v5.10.0 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/knadh/koanf/maps v0.1.2
	github.com/knadh/koanf/parsers/yaml v1.1.0 // indirect
	github.com/knadh/koanf/providers/file v1.2.1 // indirect
	github.com/labstack/gommon v0.5.0 // indirect
	github.com/mailru/easyjson v0.9.1 // indirect
	github.com/mattn/go-colorable v0.1.15 // indirect
	github.com/mattn/go-isatty v0.0.22 // indirect
	github.com/mfridman/interpolate v0.0.2 // indirect
	github.com/microsoft/go-mssqldb v1.10.0 // indirect
	github.com/minio/sha256-simd v1.0.1 // indirect
	github.com/mohae/deepcopy v0.0.0-20170929034955-c48cc78d4826 // indirect
	github.com/mr-tron/base58 v1.3.0 // indirect
	github.com/multiformats/go-varint v0.1.0 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/oasdiff/yaml v0.0.0-20260313112342-a3ea61cb4d4c // indirect
	github.com/oasdiff/yaml3 v0.0.0-20260224194419-61cd415a242b // indirect
	github.com/perimeterx/marshmallow v1.1.5 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/prometheus/client_golang v1.23.2
	github.com/prometheus/client_model v0.6.2
	github.com/prometheus/common v0.67.5 // indirect
	github.com/prometheus/procfs v0.20.1 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/rs/cors v1.11.1 // indirect
	github.com/sethvargo/go-retry v0.3.0 // indirect
	github.com/spaolacci/murmur3 v1.1.0 // indirect
	github.com/spf13/cast v1.10.0 // indirect
	github.com/stretchr/objx v0.5.3 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	github.com/valyala/fasttemplate v1.2.2 // indirect
	github.com/yuin/gopher-lua v1.1.1 // indirect
	go.lumeweb.com/queryutil v0.3.17
	go.sia.tech/jape v0.14.1 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/time v0.15.0 // indirect
	google.golang.org/grpc v1.81.1 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gorm.io/driver/postgres v1.6.0 // indirect
	gorm.io/driver/sqlserver v1.6.3 // indirect
	gorm.io/plugin/dbresolver v1.6.2 // indirect
	lukechampine.com/blake3 v1.4.1 // indirect
	modernc.org/libc v1.73.4 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
	modernc.org/sqlite v1.53.0 // indirect
)

require (
	filippo.io/edwards25519 v1.2.0 // indirect
	github.com/ClickHouse/ch-go v0.71.0 // indirect
	github.com/ClickHouse/clickhouse-go/v2 v2.46.0 // indirect
	github.com/andybalholm/brotli v1.2.1 // indirect
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.7.14 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.18.30 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.4.30 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.7.30 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.4.31 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.13 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.9.23 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.13.30 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.19.31 // indirect
	github.com/aws/aws-sdk-go-v2/service/signin v1.4.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.32.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.37.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.44.0 // indirect
	github.com/boombuler/barcode v1.1.0 // indirect
	github.com/casbin/govaluate v1.10.0 // indirect
	github.com/cenkalti/backoff/v5 v5.0.3 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/coreos/go-semver v0.3.1 // indirect
	github.com/coreos/go-systemd/v22 v22.7.0 // indirect
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/go-faster/city v1.0.1 // indirect
	github.com/go-faster/errors v0.7.1 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-openapi/swag/jsonname v0.25.5 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/gotd/contrib v0.21.1 // indirect
	github.com/hashicorp/go-version v1.8.0 // indirect
	github.com/hbollon/go-edlib v1.7.0 // indirect
	github.com/huandu/go-tls v1.0.1 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	github.com/jonboulle/clockwork v0.5.0 // indirect
	github.com/julienschmidt/httprouter v1.3.0 // indirect
	github.com/klauspost/compress v1.18.5 // indirect
	github.com/klauspost/cpuid/v2 v2.3.0 // indirect
	github.com/klauspost/reedsolomon v1.14.0 // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/montanaflynn/stats v0.9.0 // indirect
	github.com/multiformats/go-base32 v0.1.0 // indirect
	github.com/multiformats/go-base36 v0.2.0 // indirect
	github.com/paulmach/orb v0.13.0 // indirect
	github.com/pb33f/ordered-map/v2 v2.3.1 // indirect
	github.com/pierrec/lz4/v4 v4.1.26 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/quic-go/quic-go v0.60.0 // indirect
	github.com/robfig/cron/v3 v3.0.1 // indirect
	github.com/ryszard/goskiplist v0.0.0-20150312221310-2dfbae5fcf46 // indirect
	github.com/segmentio/asm v1.2.1 // indirect
	github.com/shopspring/decimal v1.4.0 // indirect
	github.com/woodsbury/decimal128 v1.4.0 // indirect
	go.etcd.io/etcd/api/v3 v3.7.0 // indirect
	go.etcd.io/etcd/client/pkg/v3 v3.7.0 // indirect
	go.lumeweb.com/gswagger v0.20.12 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.44.0 // indirect
	go.opentelemetry.io/otel/metric v1.44.0 // indirect
	go.opentelemetry.io/proto/otlp v1.10.0 // indirect
	go.shabbyrobe.org/gocovmerge v0.0.0-20230507111327-fa4f82cfbf4d // indirect
	go.sia.tech/mux v1.5.2 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.yaml.in/yaml/v2 v2.4.4 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	go.yaml.in/yaml/v4 v4.0.0-rc.2 // indirect
	golang.org/x/arch v0.24.0 // indirect
	golang.org/x/mod v0.38.0 // indirect
	golang.org/x/net v0.57.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.40.0 // indirect
	golang.org/x/tools v0.48.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20260526163538-3dc84a4a5aaa // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260526163538-3dc84a4a5aaa // indirect
	gorm.io/driver/clickhouse v0.7.0 // indirect
	lukechampine.com/frand v1.5.1 // indirect
)

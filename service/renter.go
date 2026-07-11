package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/url"
	"strings"
	"time"

	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db"
	"go.lumeweb.com/portal/db/models"
	renterInternal "go.lumeweb.com/portal/service/internal/renter"
	renterMetrics "go.lumeweb.com/portal/service/internal/renter"
	"go.opentelemetry.io/otel/attribute"
	"go.sia.tech/core/types"
	"go.sia.tech/renterd/v2/api"
	autoPilotClient "go.sia.tech/renterd/v2/autopilot"
	busClient "go.sia.tech/renterd/v2/bus/client"
	workerClient "go.sia.tech/renterd/v2/worker/client"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

var _ core.RenterService = (*RenterDefault)(nil)

func init() {
	core.RegisterService(core.ServiceInfo{
		ID: core.RENTER_SERVICE,
		Factory: func() (core.Service, []core.ContextBuilderOption, error) {
			return NewRenterService()
		},
		Metrics: renterMetrics.GetCollectors(),
	})
}

type RenterDefault struct {
	*core.BaseComponent
	busClient       *busClient.Client
	workerClient    *workerClient.Client
	autoPilotClient *autoPilotClient.Client
	clientManager   *renterInternal.ClientManager
}

func NewRenterService() (*RenterDefault, []core.ContextBuilderOption, error) {
	renter := &RenterDefault{}

	opts := core.ContextOptions(
		core.ContextWithStartupFunc(func(ctx core.Context) error {
			err := renter.init()
			if err != nil {
				return fmt.Errorf("failed to initialize renter service: %w", err)
			}

			if err != nil {
				return err
			}

			return nil
		}),
	)

	return renter, opts, nil
}

func (r *RenterDefault) ID() string {
	return core.RENTER_SERVICE
}

func (r *RenterDefault) CreateBucketIfNotExists(bucket string) error {
	return core.MetricTrack(
		renterMetrics.BucketOperationDuration.WithLabelValues(renterMetrics.LabelOperationCheck),
		renterMetrics.BucketOperationsTotal.WithLabelValues(renterMetrics.LabelOperationCheck, renterMetrics.LabelStatusError),
		func() error {
			client, err := r.getBusClient()
			if err != nil {
				return err
			}

			_, err = renterMetrics.TrackApiCallResult(
				renterMetrics.LabelClientTypeBus,
				renterMetrics.LabelEndpointBucket,
				func() (any, error) {
					return client.Bucket(context.Background(), bucket)
				},
			)

			if err == nil {
				renterMetrics.BucketOperationsTotal.WithLabelValues(renterMetrics.LabelOperationCheck, renterMetrics.LabelStatusSuccess).Inc()
				return nil
			}

			if !strings.Contains(err.Error(), api.ErrBucketNotFound.Error()) {
				return err
			}

			err = renterMetrics.TrackApiCall(
				renterMetrics.LabelClientTypeBus,
				renterMetrics.LabelEndpointBucket,
				func() error {
					return client.CreateBucket(context.Background(), bucket, api.CreateBucketOptions{
						Policy: api.BucketPolicy{
							PublicReadAccess: false,
						},
					})
				},
			)
			if err != nil {
				return err
			}

			renterMetrics.BucketOperationsTotal.WithLabelValues(renterMetrics.LabelOperationCreate, renterMetrics.LabelStatusSuccess).Inc()
			return nil
		},
	)
}

func (r *RenterDefault) UploadObject(ctx context.Context, file io.Reader, bucket string, fileName string) error {
	ctx, span := core.TraceMethod(ctx, "RenterDefault.UploadObject",
		core.WithAttributes(attribute.String("fileName", fileName)))
	defer span.End()

	return core.MetricTrack(
		renterMetrics.ObjectOperationDuration.WithLabelValues(renterMetrics.LabelOperationUpload),
		renterMetrics.ObjectOperationsTotal.WithLabelValues(renterMetrics.LabelOperationUpload, renterMetrics.LabelStatusError),
		func() error {
			client, err := r.getWorkerClient()
			if err != nil {
				return err
			}

			fileName = "/" + strings.TrimLeft(fileName, "/")
			err = renterMetrics.TrackApiCall(
				renterMetrics.LabelClientTypeWorker,
				renterMetrics.LabelEndpointObject,
				func() error {
					_, err := client.UploadObject(ctx, file, bucket, fileName, api.UploadObjectOptions{})
					return err
				},
			)

			if err != nil {
				return err
			}

			renterMetrics.ObjectOperationsTotal.WithLabelValues(renterMetrics.LabelOperationUpload, renterMetrics.LabelStatusSuccess).Inc()
			return nil
		},
	)
}

func (r *RenterDefault) init() error {
	r.clientManager = renterInternal.NewClientManager(r.Context())
	if err := r.clientManager.Start(); err != nil {
		return fmt.Errorf("failed to start client manager: %w", err)
	}

	clusterEnabled := r.Config().Config().Core.ClusterEnabled() && r.Config().Config().Core.Storage.Sia.Cluster

	if !clusterEnabled {
		addr := r.Config().Config().Core.Storage.Sia.URL
		passwd := r.Config().Config().Core.Storage.Sia.Key
		addrURL, err := url.Parse(addr)
		if err != nil {
			return err
		}

		addrURL.Path = "/api/worker"
		r.workerClient = workerClient.New(addrURL.String(), passwd)

		addrURL.Path = "/api/bus"
		r.busClient = busClient.New(addrURL.String(), passwd)

		addrURL.Path = "/api/autopilot"
		r.autoPilotClient = autoPilotClient.NewClient(addrURL.String(), passwd)

		_, stateErr := r.busClient.State(r.Context())
		if stateErr != nil {
			return fmt.Errorf("renter status check: failed to get renter state: %w", stateErr)
		}
	}

	return nil
}

func (r *RenterDefault) getBusClient() (*busClient.Client, error) {
	if !r.Config().Config().Core.ClusterEnabled() || !r.Config().Config().Core.Storage.Sia.Cluster {
		if r.busClient == nil {
			return nil, fmt.Errorf("bus client not initialized")
		}
		return r.busClient, nil
	}

	node, err := r.clientManager.GetNextNode(renterInternal.ClientTypeBus)
	if err != nil {
		return nil, fmt.Errorf("failed to get bus node: %w", err)
	}

	client := busClient.New(node.URL, r.Config().Config().Core.Storage.Sia.Key)
	return client, nil
}

func (r *RenterDefault) getWorkerClient() (*workerClient.Client, error) {
	if !r.Config().Config().Core.ClusterEnabled() || !r.Config().Config().Core.Storage.Sia.Cluster {
		if r.workerClient == nil {
			return nil, fmt.Errorf("worker client not initialized")
		}
		return r.workerClient, nil
	}

	node, err := r.clientManager.GetNextNode(renterInternal.ClientTypeWorker)
	if err != nil {
		return nil, fmt.Errorf("failed to get worker node: %w", err)
	}

	client := workerClient.New(node.URL, r.Config().Config().Core.Storage.Sia.Key)
	return client, nil
}

func (r *RenterDefault) GetObject(ctx context.Context, bucket string, fileName string, options api.DownloadObjectOptions) (*api.GetObjectResponse, error) {
	ctx, span := core.TraceMethod(ctx, "RenterDefault.GetObject",
		core.WithAttributes(attribute.String("fileName", fileName)))
	defer span.End()

	return core.MetricTrackResult(
		renterMetrics.ObjectOperationDuration.WithLabelValues(renterMetrics.LabelOperationDownload),
		renterMetrics.ObjectOperationsTotal.WithLabelValues(renterMetrics.LabelOperationDownload, renterMetrics.LabelStatusError),
		func() (*api.GetObjectResponse, error) {
			client, err := r.getWorkerClient()
			if err != nil {
				return nil, err
			}
			fileName = "/" + strings.TrimLeft(fileName, "/")
			result, err := renterMetrics.TrackApiCallResult(
				renterMetrics.LabelClientTypeWorker,
				renterMetrics.LabelEndpointObject,
				func() (*api.GetObjectResponse, error) {
					return client.GetObject(ctx, bucket, fileName, options)
				},
			)
			if err != nil {
				return result, err
			}
			renterMetrics.ObjectOperationsTotal.WithLabelValues(renterMetrics.LabelOperationDownload, renterMetrics.LabelStatusSuccess).Inc()
			return result, nil
		},
	)
}

func (r *RenterDefault) GetObjectMetadata(ctx context.Context, bucket string, fileName string) (*api.Object, error) {
	ctx, span := core.TraceMethod(ctx, "RenterDefault.GetObjectMetadata")
	defer span.End()

	return core.MetricTrackResult(
		renterMetrics.ObjectOperationDuration.WithLabelValues(renterMetrics.LabelOperationDownload),
		renterMetrics.ObjectOperationsTotal.WithLabelValues(renterMetrics.LabelOperationDownload, renterMetrics.LabelStatusError),
		func() (*api.Object, error) {
			client, err := r.getBusClient()
			if err != nil {
				return nil, err
			}
			ret, err := renterMetrics.TrackApiCallResult(
				renterMetrics.LabelClientTypeBus,
				renterMetrics.LabelEndpointObjectMetadata,
				func() (api.Object, error) {
					return client.Object(ctx, bucket, fileName, api.GetObjectOptions{})
				},
			)

			if err != nil {
				return nil, err
			}

			renterMetrics.ObjectOperationsTotal.WithLabelValues(renterMetrics.LabelOperationDownload, renterMetrics.LabelStatusSuccess).Inc()
			return &ret, nil
		},
	)
}

func (r *RenterDefault) DeleteObjectMetadata(ctx context.Context, bucket string, fileName string) error {
	ctx, span := core.TraceMethod(ctx, "RenterDefault.DeleteObjectMetadata")
	defer span.End()

	return core.MetricTrack(
		renterMetrics.ObjectOperationDuration.WithLabelValues(renterMetrics.LabelOperationDelete),
		renterMetrics.ObjectOperationsTotal.WithLabelValues(renterMetrics.LabelOperationDelete, renterMetrics.LabelStatusError),
		func() error {
			client, err := r.getBusClient()
			if err != nil {
				return err
			}
			err = renterMetrics.TrackApiCall(
				renterMetrics.LabelClientTypeBus,
				renterMetrics.LabelEndpointObjectMetadata,
				func() error {
					return client.DeleteObject(ctx, bucket, fileName)
				},
			)
			if err != nil {
				return err
			}
			renterMetrics.ObjectOperationsTotal.WithLabelValues(renterMetrics.LabelOperationDelete, renterMetrics.LabelStatusSuccess).Inc()
			return nil
		},
	)
}

func (r *RenterDefault) UploadExists(ctx context.Context, bucket string, fileName string) (bool, *models.SiaUpload, error) {
	ctx, span := core.TraceMethod(ctx, "RenterDefault.UploadExists")
	defer span.End()

	var siaUpload models.SiaUpload

	siaUpload.Bucket = bucket
	siaUpload.Key = fileName

	if err := db.RetryableComponentTransaction(r, ctx, func(db *gorm.DB) *gorm.DB {
		return db.WithContext(ctx).Model(&models.SiaUpload{}).Where(&siaUpload).First(&siaUpload)
	}); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil, nil
		}
		return false, nil, err
	}

	return true, &siaUpload, nil
}

func (r *RenterDefault) UploadObjectMultipart(ctx context.Context, params *core.MultipartUploadParams) error {
	ctx, span := core.TraceMethod(ctx, "RenterDefault.UploadObjectMultipart")
	defer span.End()

	return core.MetricTrack(
		renterMetrics.ObjectOperationDuration.WithLabelValues(renterMetrics.LabelOperationUpload),
		renterMetrics.ObjectOperationsTotal.WithLabelValues(renterMetrics.LabelOperationUpload, renterMetrics.LabelStatusError),
		func() error {
			size := params.Size
			rf := params.ReaderFactory
			bucket := params.Bucket
			fileName := params.FileName
			fileName = "/" + strings.TrimLeft(fileName, "/")

			slabSize, err := r.SlabSize(ctx)
			if err != nil {
				return err
			}

			parts := uint64(math.Ceil(float64(size) / float64(slabSize)))
			uploadParts := make([]api.MultipartCompletedPart, 0)

			var uploadId string
			start := uint64(0)

			var siaUpload models.SiaUpload

			siaUpload.Bucket = bucket
			siaUpload.Key = fileName

			err = db.RetryableComponentTransaction(r, ctx, func(db *gorm.DB) *gorm.DB {
				return db.WithContext(ctx).Model(&siaUpload).First(&siaUpload)

			})
			if err != nil {
				if !errors.Is(err, gorm.ErrRecordNotFound) {
					return err
				}
			} else {
				uploadId = siaUpload.UploadID
			}

			if len(uploadId) == 0 {
				client, err := r.getBusClient()
				if err != nil {
					return err
				}
				var upload api.MultipartCreateResponse
				upload, err = renterMetrics.TrackApiCallResult(
					renterMetrics.LabelClientTypeBus,
					renterMetrics.LabelEndpointMultipartUpload,
					func() (api.MultipartCreateResponse, error) {
						return client.CreateMultipartUpload(ctx, bucket, fileName, api.CreateMultipartOptions{})
					},
				)
				if err != nil {
					return err
				}

				uploadId = upload.UploadID
				siaUpload.UploadID = uploadId
				if err = db.RetryableComponentTransaction(r, ctx, func(db *gorm.DB) *gorm.DB {
					return db.WithContext(ctx).Create(&siaUpload)
				}); err != nil {
					return err
				}
			} else {
				client, err := r.getBusClient()
				if err != nil {
					return err
				}
				existing, err := renterMetrics.TrackApiCallResult(
					renterMetrics.LabelClientTypeBus,
					renterMetrics.LabelEndpointMultipartUpload,
					func() (api.MultipartListPartsResponse, error) {
						return client.MultipartUploadParts(ctx, bucket, fileName, uploadId, 0, 0)
					},
				)

				if err != nil {
					uploadId = ""
				} else {
					for _, part := range existing.Parts {
						if uint64(part.Size) != slabSize {
							break
						}
						partNumber := part.PartNumber
						uploadParts = append(uploadParts, api.MultipartCompletedPart{
							PartNumber: partNumber,
							ETag:       part.ETag,
						})
					}

					if len(uploadParts) > 0 {
						start = uint64(len(uploadParts)) - 1
					}
				}
			}

			reader, err := rf(uint(start*slabSize), uint(0))
			if err != nil {
				return err
			}

			defer func(reader io.ReadCloser) {
				err := reader.Close()
				if err != nil {
					r.Logger().Error("error closing reader", zap.Error(err))
				}
			}(reader)

			for i := start; i < parts; i++ {
				lr := io.LimitReader(reader, int64(slabSize))
				partNumber := int(i + 1)
				offset := int(i * slabSize)

				opts := api.UploadMultipartUploadPartOptions{}
				opts.EncryptionOffset = &offset

				client, err := r.getWorkerClient()
				if err != nil {
					return err
				}
				ret, err := renterMetrics.TrackApiCallResult(
					renterMetrics.LabelClientTypeWorker,
					renterMetrics.LabelEndpointMultipartUploadPart,
					func() (*api.UploadMultipartUploadPartResponse, error) {
						return client.UploadMultipartUploadPart(ctx, lr, bucket, fileName, uploadId, partNumber, opts)
					},
				)
				if err != nil {
					return err
				}

				uploadParts = append(uploadParts, api.MultipartCompletedPart{
					PartNumber: partNumber,
					ETag:       ret.ETag,
				})

				siaUpload.UpdatedAt = time.Now()

				if err = db.RetryableComponentTransaction(r, ctx, func(db *gorm.DB) *gorm.DB {
					return db.WithContext(ctx).Model(&siaUpload).Save(&siaUpload)
				}); err != nil {
					return err
				}
			}

			client, err := r.getBusClient()
			if err != nil {
				return err
			}
			err = renterMetrics.TrackApiCall(
				renterMetrics.LabelClientTypeBus,
				renterMetrics.LabelEndpointMultipartUpload,
				func() error {
					_, err := client.CompleteMultipartUpload(ctx, bucket, fileName, uploadId, uploadParts, api.CompleteMultipartOptions{})
					return err
				},
			)
			if err != nil {
				return err
			}

			if err = db.RetryableComponentTransaction(r, ctx, func(db *gorm.DB) *gorm.DB {
				return db.WithContext(ctx).Delete(&siaUpload)
			}); err != nil {
				return err
			}

			renterMetrics.ObjectOperationsTotal.WithLabelValues(renterMetrics.LabelOperationUpload, renterMetrics.LabelStatusSuccess).Inc()
			return nil
		},
	)
}

func (r *RenterDefault) DeleteObject(ctx context.Context, bucket string, fileName string) error {
	ctx, span := core.TraceMethod(ctx, "RenterDefault.DeleteObject",
		core.WithAttributes(attribute.String("fileName", fileName)))
	defer span.End()

	return core.MetricTrack(
		renterMetrics.ObjectOperationDuration.WithLabelValues(renterMetrics.LabelOperationDelete),
		renterMetrics.ObjectOperationsTotal.WithLabelValues(renterMetrics.LabelOperationDelete, renterMetrics.LabelStatusError),
		func() error {
			client, err := r.getWorkerClient()
			if err != nil {
				return err
			}
			err = renterMetrics.TrackApiCall(
				renterMetrics.LabelClientTypeWorker,
				renterMetrics.LabelEndpointObject,
				func() error {
					return client.DeleteObject(ctx, bucket, fileName)
				},
			)
			if err != nil {
				return err
			}
			renterMetrics.ObjectOperationsTotal.WithLabelValues(renterMetrics.LabelOperationDelete, renterMetrics.LabelStatusSuccess).Inc()
			return nil
		},
	)
}

func (r *RenterDefault) UpdateGougingSettings(ctx context.Context, settings api.GougingSettings) error {
	ctx, span := core.TraceMethod(ctx, "RenterDefault.UpdateGougingSettings")
	defer span.End()

	return core.MetricTrack(
		renterMetrics.ApiLatency.WithLabelValues(renterMetrics.LabelClientTypeBus, renterMetrics.LabelEndpointGouging),
		renterMetrics.ApiRequestsTotal.WithLabelValues(renterMetrics.LabelClientTypeBus, renterMetrics.LabelEndpointGouging, renterMetrics.LabelStatusError),
		func() error {
			client, err := r.getBusClient()
			if err != nil {
				return err
			}
			err = renterMetrics.TrackApiCall(
				renterMetrics.LabelClientTypeBus,
				renterMetrics.LabelEndpointGouging,
				func() error {
					return client.UpdateGougingSettings(ctx, settings)
				},
			)
			if err != nil {
				return err
			}
			renterMetrics.ApiRequestsTotal.WithLabelValues(renterMetrics.LabelClientTypeBus, renterMetrics.LabelEndpointGouging, renterMetrics.LabelStatusSuccess).Inc()
			return nil
		},
	)
}

func (r *RenterDefault) GougingSettings(ctx context.Context) (api.GougingSettings, error) {
	ctx, span := core.TraceMethod(ctx, "RenterDefault.GougingSettings")
	defer span.End()

	settings, err := core.MetricTrackResult(
		renterMetrics.ApiLatency.WithLabelValues(renterMetrics.LabelClientTypeBus, renterMetrics.LabelEndpointGouging),
		renterMetrics.ApiRequestsTotal.WithLabelValues(renterMetrics.LabelClientTypeBus, renterMetrics.LabelEndpointGouging, renterMetrics.LabelStatusError),
		func() (api.GougingSettings, error) {
			client, err := r.getBusClient()
			if err != nil {
				return api.GougingSettings{}, err
			}
			return renterMetrics.TrackApiCallResult(
				renterMetrics.LabelClientTypeBus,
				renterMetrics.LabelEndpointGouging,
				func() (api.GougingSettings, error) {
					return client.GougingSettings(ctx)
				},
			)
		},
	)
	if err != nil {
		renterMetrics.GougingCompliance.Set(0)
		return settings, err
	}
	renterMetrics.GougingCompliance.Set(1)
	renterMetrics.ApiRequestsTotal.WithLabelValues(renterMetrics.LabelClientTypeBus, renterMetrics.LabelEndpointGouging, renterMetrics.LabelStatusSuccess).Inc()
	return settings, nil
}

func (r *RenterDefault) SlabSize(ctx context.Context) (uint64, error) {
	ctx, span := core.TraceMethod(ctx, "RenterDefault.SlabSize")
	defer span.End()

	return core.MetricTrackResult(
		renterMetrics.ApiLatency.WithLabelValues(renterMetrics.LabelClientTypeBus, renterMetrics.LabelEndpointUploadSettings),
		renterMetrics.ApiRequestsTotal.WithLabelValues(renterMetrics.LabelClientTypeBus, renterMetrics.LabelEndpointUploadSettings, renterMetrics.LabelStatusError),
		func() (uint64, error) {
			client, err := r.getBusClient()
			if err != nil {
				return 0, err
			}

			uploadSettings, err := renterMetrics.TrackApiCallResult(
				renterMetrics.LabelClientTypeBus,
				renterMetrics.LabelEndpointUploadSettings,
				func() (api.UploadSettings, error) {
					return client.UploadSettings(ctx)
				},
			)
			if err != nil {
				return 0, err
			}

			renterMetrics.ApiRequestsTotal.WithLabelValues(renterMetrics.LabelClientTypeBus, renterMetrics.LabelEndpointUploadSettings, renterMetrics.LabelStatusSuccess).Inc()
			return uploadSettings.Redundancy.SlabSizeNoRedundancy(), nil
		},
	)
}

func (r *RenterDefault) Host(ctx context.Context, host types.PublicKey) (api.Host, error) {
	ctx, span := core.TraceMethod(ctx, "RenterDefault.Host")
	defer span.End()

	return core.MetricTrackResult(
		renterMetrics.ApiLatency.WithLabelValues(renterMetrics.LabelClientTypeBus, renterMetrics.LabelEndpointHost),
		renterMetrics.ApiRequestsTotal.WithLabelValues(renterMetrics.LabelClientTypeBus, renterMetrics.LabelEndpointHost, renterMetrics.LabelStatusError),
		func() (api.Host, error) {
			client, err := r.getBusClient()
			if err != nil {
				return api.Host{}, err
			}
			result, err := renterMetrics.TrackApiCallResult(
				renterMetrics.LabelClientTypeBus,
				renterMetrics.LabelEndpointHost,
				func() (api.Host, error) {
					return client.Host(ctx, host)
				},
			)
			if err != nil {
				return result, err
			}
			renterMetrics.ApiRequestsTotal.WithLabelValues(renterMetrics.LabelClientTypeBus, renterMetrics.LabelEndpointHost, renterMetrics.LabelStatusSuccess).Inc()
			return result, nil
		},
	)
}

func (r *RenterDefault) ConsensusState(ctx context.Context) (api.ConsensusState, error) {
	ctx, span := core.TraceMethod(ctx, "RenterDefault.ConsensusState")
	defer span.End()

	return core.MetricTrackResult(
		renterMetrics.ApiLatency.WithLabelValues(renterMetrics.LabelClientTypeBus, renterMetrics.LabelEndpointConsensus),
		renterMetrics.ApiRequestsTotal.WithLabelValues(renterMetrics.LabelClientTypeBus, renterMetrics.LabelEndpointConsensus, renterMetrics.LabelStatusError),
		func() (api.ConsensusState, error) {
			client, err := r.getBusClient()
			if err != nil {
				return api.ConsensusState{}, err
			}
			result, err := renterMetrics.TrackApiCallResult(
				renterMetrics.LabelClientTypeBus,
				renterMetrics.LabelEndpointConsensus,
				func() (api.ConsensusState, error) {
					return client.ConsensusState(ctx)
				},
			)
			if err != nil {
				return result, err
			}
			renterMetrics.ApiRequestsTotal.WithLabelValues(renterMetrics.LabelClientTypeBus, renterMetrics.LabelEndpointConsensus, renterMetrics.LabelStatusSuccess).Inc()
			return result, nil
		},
	)
}

func (r *RenterDefault) RecommendedFee(ctx context.Context) (types.Currency, error) {
	ctx, span := core.TraceMethod(ctx, "RenterDefault.RecommendedFee")
	defer span.End()

	fee, err := core.MetricTrackResult(
		renterMetrics.ApiLatency.WithLabelValues(renterMetrics.LabelClientTypeBus, renterMetrics.LabelEndpointFee),
		renterMetrics.ApiRequestsTotal.WithLabelValues(renterMetrics.LabelClientTypeBus, renterMetrics.LabelEndpointFee, renterMetrics.LabelStatusError),
		func() (types.Currency, error) {
			client, err := r.getBusClient()
			if err != nil {
				return types.Currency{}, err
			}
			return renterMetrics.TrackApiCallResult(
				renterMetrics.LabelClientTypeBus,
				renterMetrics.LabelEndpointFee,
				func() (types.Currency, error) {
					return client.RecommendedFee(ctx)
				},
			)
		},
	)
	if err != nil {
		renterMetrics.RecommendedFee.Set(0)
		return fee, err
	}
	renterMetrics.ApiRequestsTotal.WithLabelValues(renterMetrics.LabelClientTypeBus, renterMetrics.LabelEndpointFee, renterMetrics.LabelStatusSuccess).Inc()
	renterMetrics.RecommendedFee.Set(float64(fee.Lo))
	return fee, nil
}

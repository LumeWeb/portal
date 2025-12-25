package service

import (
	"context"

	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/service/internal/otp"
)

var _ core.OTPService = (*OTPServiceDefault)(nil)

func init() {
	core.RegisterService(core.ServiceInfo{
		ID:      core.OTP_SERVICE,
		Factory: NewOTPService,
		Depends: []string{core.USER_SERVICE},
		Metrics: otp.GetCollectors(),
	})
}

type OTPServiceDefault struct {
	user core.UserService
	core.Service
}

func NewOTPService() (core.Service, []core.ContextBuilderOption, error) {
	otp := &OTPServiceDefault{}

	opts := core.ContextOptions(
		core.ContextWithStartupFunc(func(ctx core.Context) error {
			otp.user = core.GetService[core.UserService](ctx, core.USER_SERVICE)
			return nil
		}),
	)

	return otp, opts, nil
}

func (o OTPServiceDefault) ID() string {
	return core.OTP_SERVICE
}

func (o OTPServiceDefault) OTPGenerate(ctx context.Context, userId uint) (string, error) {
	ctx, span := core.TraceMethod(ctx, "OTPServiceDefault.OTPGenerate")
	defer span.End()

	result, err := core.MetricTrackResult(
		otp.OTPOperation.WithLabelValues(otp.LabelOpGenerate),
		otp.OTPFailed.WithLabelValues(otp.LabelOpGenerate),
		func() (string, error) {
			exists, user, err := o.user.AccountExists(ctx, userId)

			if !exists || err != nil {
				if err == nil {
					err = core.NewAccountError(core.ErrKeyUserNotFound, nil)
				}
				return "", err
			}

			otpSecret, otpErr := core.TOTPGenerate(user.Email, o.Config().Config().Core.Domain)
			if otpErr != nil {
				return "", core.NewAccountError(core.ErrKeyOTPGenerationFailed, otpErr)
			}

			err = o.user.UpdateAccountInfo(ctx, user.ID, map[string]interface{}{"otp_secret": otpSecret})

			if err != nil {
				return "", err
			}

			return otpSecret, nil
		},
	)

	if err == nil {
		otp.OTPsGenerated.WithLabelValues(otp.LabelOpGenerate).Inc()
	}
	return result, err
}

func (o OTPServiceDefault) OTPVerify(ctx context.Context, userId uint, code string) (bool, error) {
	ctx, span := core.TraceMethod(ctx, "OTPServiceDefault.OTPVerify")
	defer span.End()

	result, err := core.MetricTrackResult(
		otp.OTPOperation.WithLabelValues(otp.LabelOpVerify),
		otp.OTPFailed.WithLabelValues(otp.LabelOpVerify),
		func() (bool, error) {
			exists, user, err := o.user.AccountExists(ctx, userId)

			if !exists || err != nil {
				if err == nil {
					err = core.NewAccountError(core.ErrKeyUserNotFound, nil)
				}
				return false, err
			}

			valid := core.TOTPValidate(user.OTPSecret, code)
			if !valid {
				return false, nil
			}

			return true, nil
		},
	)

	if err == nil && result {
		otp.OTPsVerified.WithLabelValues(otp.LabelOpVerify).Inc()
	}
	return result, err
}

func (o OTPServiceDefault) OTPEnable(ctx context.Context, userId uint, code string) error {
	ctx, span := core.TraceMethod(ctx, "OTPServiceDefault.OTPEnable")
	defer span.End()

	return core.MetricTrack(
		otp.OTPOperation.WithLabelValues(otp.LabelOpEnable),
		otp.OTPFailed.WithLabelValues(otp.LabelOpEnable),
		func() error {
			verify, err := o.OTPVerify(ctx, userId, code)
			if err != nil {
				return err
			}

			if !verify {
				return core.ErrInvalidOTPCode
			}

			err = o.user.UpdateAccountInfo(ctx, userId, map[string]interface{}{"otp_enabled": true})
			if err == nil {
				otp.OTPEnabled.WithLabelValues(otp.LabelOpEnable).Inc()
			}
			return err
		},
	)
}

func (o OTPServiceDefault) OTPDisable(ctx context.Context, userId uint) error {
	ctx, span := core.TraceMethod(ctx, "OTPServiceDefault.OTPDisable")
	defer span.End()

	return core.MetricTrack(
		otp.OTPOperation.WithLabelValues(otp.LabelOpDisable),
		otp.OTPFailed.WithLabelValues(otp.LabelOpDisable),
		func() error {
			exists, _, err := o.user.AccountExists(ctx, userId)

			if !exists || err != nil {
				if err == nil {
					err = core.NewAccountError(core.ErrKeyUserNotFound, nil)
				}
				return err
			}

			err = o.user.UpdateAccountInfo(ctx, userId, map[string]interface{}{"otp_enabled": false, "otp_secret": ""})
			if err == nil {
				otp.OTPDisabled.WithLabelValues(otp.LabelOpDisable).Inc()
			}
			return err
		},
	)
}

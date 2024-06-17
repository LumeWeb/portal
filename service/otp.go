package service

import (
	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db/models"
	"gorm.io/gorm"
)

var _ core.OTPService = (*OTPServiceDefault)(nil)

func init() {
	core.RegisterService(core.ServiceInfo{
		ID: core.OTP_SERVICE,
		Factory: func() (core.Service, []core.ContextBuilderOption, error) {
			return NewOTPService()
		},
		Depends: []string{core.USER_SERVICE},
	})
}

type OTPServiceDefault struct {
	ctx    core.Context
	config config.Manager
	db     *gorm.DB
	user   core.UserService
}

func NewOTPService() (*OTPServiceDefault, []core.ContextBuilderOption, error) {
	otp := &OTPServiceDefault{}

	opts := core.ContextOptions(
		core.ContextWithStartupFunc(func(ctx core.Context) error {
			otp.ctx = ctx
			otp.config = ctx.Config()
			otp.db = ctx.DB()
			otp.user = ctx.Service(core.USER_SERVICE).(core.UserService)
			return nil
		}),
	)

	return otp, opts, nil
}

func (o OTPServiceDefault) OTPGenerate(userId uint) (string, error) {
	exists, user, err := o.user.AccountExists(userId)

	if !exists || err != nil {
		return "", err
	}

	otp, otpErr := core.TOTPGenerate(user.Email, o.config.Config().Core.Domain)
	if otpErr != nil {
		return "", core.NewAccountError(core.ErrKeyOTPGenerationFailed, otpErr)
	}

	err = o.user.UpdateAccountInfo(user.ID, models.User{OTPSecret: otp})

	if err != nil {
		return "", err
	}

	return otp, nil
}

func (o OTPServiceDefault) OTPVerify(userId uint, code string) (bool, error) {
	exists, user, err := o.user.AccountExists(userId)

	if !exists || err != nil {
		return false, err
	}

	valid := core.TOTPValidate(user.OTPSecret, code)
	if !valid {
		return false, nil
	}

	return true, nil
}

func (o OTPServiceDefault) OTPEnable(userId uint, code string) error {
	verify, err := o.OTPVerify(userId, code)
	if err != nil {
		return err
	}

	if !verify {
		return core.ErrInvalidOTPCode
	}

	return o.user.UpdateAccountInfo(userId, models.User{OTPEnabled: true})
}

func (o OTPServiceDefault) OTPDisable(userId uint) error {
	return o.user.UpdateAccountInfo(userId, models.User{OTPEnabled: false, OTPSecret: ""})
}

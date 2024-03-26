package account

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Token string `json:"token"`
	Otp   bool   `json:"otp"`
}

type RegisterRequest struct {
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Email     string `json:"email"`
	Password  string `json:"password"`
}

type OTPGenerateResponse struct {
	OTP string `json:"otp"`
}

type OTPVerifyRequest struct {
	OTP string `json:"otp"`
}

type OTPValidateRequest struct {
	OTP string `json:"otp"`
}
type OTPDisableRequest struct {
	Password string `json:"password"`
}
type VerifyEmailRequest struct {
	Email string `json:"email"`
	Token string `json:"token"`
}
type PasswordResetRequest struct {
	Email string `json:"email"`
}
type PasswordResetVerifyRequest struct {
	Email    string `json:"email"`
	Token    string `json:"token"`
	Password string `json:"password"`
}

type PongResponse struct {
	Ping  string `json:"ping"`
	Token string `json:"token"`
}
type AccountInfoResponse struct {
	ID        uint   `json:"id"`
	Email     string `json:"email"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

type UploadLimitResponse struct {
	Limit uint64 `json:"limit"`
}
type UpdateEmailRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}
type UpdatePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

type MetaResponse struct {
	Domain string `json:"domain"`
}

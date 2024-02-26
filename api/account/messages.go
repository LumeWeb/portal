package account

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
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

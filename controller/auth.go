package controller

import (
	"git.lumeweb.com/LumeWeb/portal/controller/request"
	"git.lumeweb.com/LumeWeb/portal/controller/response"
	"git.lumeweb.com/LumeWeb/portal/middleware"
	"git.lumeweb.com/LumeWeb/portal/service/auth"
	"github.com/kataras/iris/v12"
)

type AuthController struct {
	Controller
}

// PostLogin handles the POST /api/auth/login request to authenticate a user and return a JWT token.
func (a *AuthController) PostLogin() {
	ri, success := tryParseRequest(request.LoginRequest{}, a.Ctx)
	if !success {
		return
	}

	r, _ := ri.(*request.LoginRequest)

	token, err := auth.LoginWithPassword(r.Email, r.Password)

	if err != nil {
		if err == auth.ErrFailedGenerateToken {
			a.Ctx.StopWithError(iris.StatusInternalServerError, err)
		} else {
			a.Ctx.StopWithError(iris.StatusUnauthorized, err)
		}
		return
	}

	a.respondJSON(&response.LoginResponse{Token: token})
}

// PostChallenge handles the POST /api/auth/pubkey/challenge request to generate a challenge for a user's public key.
func (a *AuthController) PostPubkeyChallenge() {
	ri, success := tryParseRequest(request.PubkeyChallengeRequest{}, a.Ctx)
	if !success {
		return
	}

	r, _ := (ri).(*request.PubkeyChallengeRequest)

	challenge, err := auth.GeneratePubkeyChallenge(r.Pubkey)
	if err != nil {
		if err == auth.ErrFailedGenerateKeyChallenge {
			a.Ctx.StopWithError(iris.StatusInternalServerError, err)
		} else {
			a.Ctx.StopWithError(iris.StatusUnauthorized, err)
		}
		return
	}

	a.respondJSON(&response.ChallengeResponse{Challenge: challenge})
}

// PostKeyLogin handles the POST /api/auth/pubkey/login request to authenticate a user using a public key challenge and return a JWT token.
func (a *AuthController) PostPubkeyLogin() {
	ri, success := tryParseRequest(request.PubkeyLoginRequest{}, a.Ctx)
	if !success {
		return
	}

	r, _ := ri.(*request.PubkeyLoginRequest)

	token, err := auth.LoginWithPubkey(r.Pubkey, r.Challenge, r.Signature)

	if err != nil {
		if err == auth.ErrFailedGenerateKeyChallenge || err == auth.ErrFailedGenerateToken || err == auth.ErrFailedSaveToken {
			a.Ctx.StopWithError(iris.StatusInternalServerError, err)
		} else {
			a.Ctx.StopWithError(iris.StatusUnauthorized, err)
		}
		return
	}

	a.respondJSON(&response.LoginResponse{Token: token})

}

// PostLogout handles the POST /api/auth/logout request to invalidate a JWT token.
func (a *AuthController) PostLogout() {
	ri, success := tryParseRequest(request.LogoutRequest{}, a.Ctx)
	if !success {
		return
	}

	r, _ := ri.(*request.LogoutRequest)

	err := auth.Logout(r.Token)

	if err != nil {
		a.Ctx.StopWithError(iris.StatusBadRequest, err)
		return
	}

	// Return a success response to the client.
	a.Ctx.StatusCode(iris.StatusNoContent)
}

func (a *AuthController) GetStatus() {
	middleware.VerifyJwt(a.Ctx)

	if a.Ctx.IsStopped() {
		return
	}

	a.respondJSON(&response.AuthStatusResponse{Status: true})
}

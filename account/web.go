package account

import "go.sia.tech/jape"

func SendJWT(jc jape.Context, jwt string) {
	jc.ResponseWriter.Header().Set("Authorization", "Bearer "+jwt)
}

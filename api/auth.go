package api

import (
	"time"
	"fmt"
	"strconv"
	"encoding/json"
	"cogged/log"
	req "cogged/requests"
	res "cogged/responses"
	svc "cogged/services"
	sec "cogged/security"
	state "cogged/state"
)

type AuthAPI struct {
	Configuration	*svc.Config
	Database		*svc.DB
	SecretKey		string
	TokenExpiry		int64
}

func NewAuthAPI(config *svc.Config, db *svc.DB, key string) *AuthAPI {
	a := &AuthAPI{
		Configuration: config,
		Database: db,
		SecretKey: key,
	}
	confTime := config.Get("auth.tokenexpiry")
	a.TokenExpiry = getTokenExpiry(confTime)
	return a
}

func getTokenExpiry(confTime string) int64 {
	expTime, convErr := strconv.ParseInt(confTime, 10, 64)
	if convErr != nil || expTime <= 0 { expTime = 600 }
	return expTime
}


func newTokenId() string {
	b,_ := sec.GenerateRandomBytes(3)
	return sec.B64Encode(b)
}

func (h *AuthAPI) tokenResponse(uid, role, tokenId string) *res.TokenResponse {
	// issue token
	issuedAt := time.Now().Unix()
	timestamp := fmt.Sprintf("%d", issuedAt)
	tok := sec.ConstructToken(uid, role, tokenId, timestamp, h.SecretKey)
	tr := &res.TokenResponse{
		Token: tok,
		Expires: int(h.TokenExpiry),
	}
	return tr
}


func (h *AuthAPI) HandleRequest(handlerKey, param, body string, uad *sec.UserAuthData) (string, error) {
	ud := req.UnpackData{ UAD: uad }

	switch handlerKey {
		case "POST login":
			lr := &req.LoginRequest{}
			if berr := req.BindToRequest[req.LoginRequest](body, lr, ud); berr != nil {
				return "", &APIError{Info: berr.Error(), StatusCode: 400}
			}

			// verify creds
			dbres, err := h.Database.QueryUser(lr.Username);
			if err != nil || dbres.User == nil || !sec.VerifyPasswordHash(*dbres.User.PasswordHash, lr.Password) {
				return "", &APIError{Info: "invalid login", StatusCode: 401}
			}

			loggedInUser := dbres.User
			log.Debug("loggedInUser.PasswordHash",*loggedInUser.PasswordHash)

			nti := newTokenId()
			state.UsmAddTokenId(loggedInUser.Uid, nti)
			tr := h.tokenResponse(loggedInUser.Uid, *loggedInUser.Role, nti)
			resp, err := json.Marshal(tr)
			return string(resp), err

		case "POST logout":
			state.UsmDeleteTokenId(uad.Uid, uad.TokenId)
			return "{}", nil

		case "GET refresh":
			tr := h.tokenResponse(uad.Uid, uad.Role, uad.TokenId)
			resp, err := json.Marshal(tr)
			return string(resp), err

		case "GET check":
			return "{}", nil

		case "GET clientconfig":
			d := map[string]string{"config": h.Configuration.Get("client.config")}
			return MarshalJSON[map[string]string](&d, uad), nil
		
	}
	return "", &APIError{Info: "not found", StatusCode: 404}
}

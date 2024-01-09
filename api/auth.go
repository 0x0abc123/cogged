package api

import (
	"encoding/json"
	"cogged/log"
	req "cogged/requests"
	res "cogged/responses"
	svc "cogged/services"
	sec "cogged/security"
)

type AuthAPI struct {
	Configuration	*svc.Config
	Database		*svc.DB
	SecretKey		string
}

func (h *AuthAPI) HandleRequest(handlerKey, param, body string, uad *sec.UserAuthData) (string, error) {
	log.Debug("AuthAPI: HandleRequest", handlerKey, param, body)

	ud := req.UnpackData{ UAD: uad }

	switch handlerKey {
		case "POST login":
			lr := &req.LoginRequest{}
			if berr := req.BindToRequest[req.LoginRequest](body, lr, ud); berr != nil {
				return "", &APIError{Info: berr.Error()}
			}

			// verify creds
			dbres, err := h.Database.QueryUser(lr.Username);
			if err != nil || dbres.User == nil || !sec.VerifyPasswordHash(*dbres.User.PasswordHash, lr.Password) {
				return "", &APIError{Info: "invalid login"}
			}

			loggedInUser := dbres.User
			log.Debug("loggedInUser.PasswordHash",*loggedInUser.PasswordHash)

			// issue token
			tok := sec.ConstructToken(loggedInUser.Uid, *loggedInUser.Role, h.SecretKey)
			tr := &res.TokenResponse{
				Token: tok,
				Expires: 600, // TODO: from config file
			}			
			resp, err := json.Marshal(tr)
			return string(resp), err
			
		case "GET check":
			return "OK", nil
	}
	return "", &APIError{Info: "not found"}
}
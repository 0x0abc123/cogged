package api

import (
	"cogged/log"
	cm "cogged/models"
	svc "cogged/services"
	sec "cogged/security"
	req "cogged/requests"
	res "cogged/responses"
)

type AdminAPI struct {
	Configuration	*svc.Config
	Database		*svc.DB
}

func (h *AdminAPI) HandleRequest(handlerKey, param, body string, uad *sec.UserAuthData) (string, error) {
	log.Debug("AdminAPI: HandleRequest", handlerKey, param, body)

	ud := req.UnpackData{ UAD: uad }

	switch handlerKey {

		case "PUT user":
			r := &req.CreateUserRequest{}
			if berr := req.BindToRequest[req.CreateUserRequest](body, r, ud); berr != nil {
				return "", &APIError{Info: berr.Error()}
			}

			user := cm.GraphUser{ 
				GraphBase: cm.GraphBase{Uid: "newuser"},
				Username: &r.Username,
				Data: r.UserData,
				InternalData: r.Internal,
				Role: &r.Role,
			}
			pwdHash := sec.GeneratePasswordHash(r.Password)
			user.PasswordHash = &pwdHash
			cr,_ := h.Database.UpsertUsers(&[]*cm.GraphUser{&user})
			return MarshalJSON[res.CoggedResponse](cr, uad), nil

		case "PATCH users":
			r := &req.UsersRequest{}
			if berr := req.BindToRequest[req.UsersRequest](body, r, ud); berr != nil {
				return "", &APIError{Info: berr.Error()}
			}

			usersToUpdate := r.Users
			//ValidateUids
			for _, u := range *usersToUpdate {
				if !svc.ValidateUid(u.Uid) {
					return "", &APIError{Info: "bad uid"}
				}
				l := len(*u.PasswordHash)
				if l > 0 {
					if l <= req.MIN_USER_PASS_LENGTH {
						return "", &APIError{Info: "password does not meet min length"}
					}
					pwdHash := sec.GeneratePasswordHash(*u.PasswordHash)
					u.PasswordHash = &pwdHash
				} 
			}
			cr,_ := h.Database.UpsertUsers(usersToUpdate)
			return MarshalJSON[res.CoggedResponse](cr, uad), nil
	}
	return "", &APIError{Info: "not found"}
}
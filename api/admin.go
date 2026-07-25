package api

import (
	cm "cogged/models"
	req "cogged/requests"
	res "cogged/responses"
	sec "cogged/security"
	svc "cogged/services"
)

type AdminAPI struct {
	Configuration *svc.Config
	Database      *svc.DB
}

func NewAdminAPI(config *svc.Config, db *svc.DB) *AdminAPI {
	a := &AdminAPI{
		Configuration: config,
		Database:      db,
	}
	return a
}

func (h *AdminAPI) HandleRequest(handlerKey, param, body string, uad *sec.UserAuthData) (string, error) {
	ud := req.UnpackData{UAD: uad}

	switch handlerKey {

	case "PUT user":
		r := &req.CreateUserRequest{}
		if berr := req.BindToRequest[req.CreateUserRequest](body, r, ud); berr != nil {
			return "", &APIError{Info: berr.Error(), StatusCode: 400}
		}

		user := cm.GraphUser{
			GraphBase:    cm.GraphBase{Uid: "newuser"},
			Username:     &r.Username,
			Data:         r.UserData,
			InternalData: r.Internal,
			Role:         &r.Role,
		}
		pwdHash := sec.GeneratePasswordHash(r.Password)
		user.PasswordHash = &pwdHash
		cr, _ := h.Database.UpsertUsers(&[]*cm.GraphUser{&user})
		return MarshalJSON[res.CoggedResponse](cr, uad), nil

	case "PATCH users":
		r := &req.UsersRequest{}
		if berr := req.BindToRequest[req.UsersRequest](body, r, ud); berr != nil {
			return "", &APIError{Info: berr.Error(), StatusCode: 400}
		}

		usersToUpdate := r.Users
		if aerr := prepareUsersForUpdate(*usersToUpdate); aerr != nil {
			return "", aerr
		}
		cr, _ := h.Database.UpsertUsers(usersToUpdate)
		return MarshalJSON[res.CoggedResponse](cr, uad), nil
	}
	return "", &APIError{Info: "not found", StatusCode: 404}
}

// prepareUsersForUpdate validates each user's uid and hashes any supplied password in
// place. A user with no password (nil or empty PasswordHash) is left unchanged rather
// than dereferencing a nil pointer. Returns an *APIError on the first invalid user.
func prepareUsersForUpdate(users []*cm.GraphUser) *APIError {
	for _, u := range users {
		if !svc.ValidateUid(u.Uid) {
			return &APIError{Info: "bad uid", StatusCode: 400}
		}
		if u.PasswordHash != nil && len(*u.PasswordHash) > 0 {
			if len(*u.PasswordHash) <= req.MIN_USER_PASS_LENGTH {
				return &APIError{Info: "password does not meet min length", StatusCode: 400}
			}
			pwdHash := sec.GeneratePasswordHash(*u.PasswordHash)
			u.PasswordHash = &pwdHash
		}
	}
	return nil
}

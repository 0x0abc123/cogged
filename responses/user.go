package responses

import (
	cm "cogged/models"
	sec "cogged/security"
)

type UserResponse struct {
	User 	*cm.GraphUser 	`json:"user"`
	Error 	string 			`json:"error,omitempty"`
}


func UserResponseFromError(e string) *UserResponse {
	return &UserResponse{Error: e}
}


func UserResponseFromUser(u *cm.GraphUser) *UserResponse {
	return &UserResponse{User: u}
}


func (resp *UserResponse) AuthzDataPack(uad *sec.UserAuthData) {
	if resp.User != nil {
		resp.User.AuthzDataPack(uad)
	}
}
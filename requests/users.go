package requests

import (
	cm "cogged/models"
	sec "cogged/security"
)

type UsersRequest struct {

	Users *[]*cm.GraphUser	`json:"users,omitempty"`

}


// not applicable, as the request is admin only and actual UIDs are accepted
func (req *UsersRequest) AuthzDataUnpack(uad sec.UserAuthData, permissionsRequired string) bool {
	return true
}


func (u *UsersRequest) Validate() bool {
	return !(u.Users == nil || len(*(u.Users)) < 1)	
}


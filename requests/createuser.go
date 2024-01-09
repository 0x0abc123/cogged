package requests

import (
	"strings"
	sec "cogged/security"
)

const MIN_USER_PASS_LENGTH = 4

type CreateUserRequest struct {
	Username	string		`json:"username"`
	Password	string		`json:"password"`
	UserData	*string		`json:"us,omitempty"`
	Internal	*string		`json:"intd,omitempty"`
	Role		string		`json:"role"`
}


// not applicable, as the request is admin only and actual UIDs are accepted
func (req *CreateUserRequest) AuthzDataUnpack(uad sec.UserAuthData, permissionsRequired string) bool {
	return true
}


func (c *CreateUserRequest) Validate() bool {
	return len(c.Username) > 0 && len(c.Password) > MIN_USER_PASS_LENGTH && !strings.HasPrefix(c.Username,"~")
}
package requests

import (
	"strings"
	sec "cogged/security"
)

type LoginRequest struct {
	Username	string		`json:"username"`
	Password	string		`json:"password"`
}


// not applicable, as the request is an unauthenticated one that doesnt identify anything with UIDs
func (req *LoginRequest) AuthzDataUnpack(uad sec.UserAuthData, permissionsRequired string) bool {
	return true
}


func (lr *LoginRequest) Validate() bool {
	return len(lr.Username) > 0 && !strings.HasPrefix(lr.Username,"~")
}
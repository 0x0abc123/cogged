package requests

import sec "cogged/security"

type AuthzDataUnpacker interface {
	AuthzDataUnpack(uad sec.UserAuthData, permissionsRequired string) bool 
}
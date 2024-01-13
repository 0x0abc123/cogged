package responses

import sec "cogged/security"

type AuthzDataPacker interface {
	AuthzDataPack(UAD *sec.UserAuthData) 
}
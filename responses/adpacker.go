// Package responses defines the outbound JSON response DTOs. The AuthzDataPacker seam
// stamps signed AuthzData onto nodes/users so real UIDs never leave the server unprotected.
package responses

import sec "cogged/security"

type AuthzDataPacker interface {
	AuthzDataPack(UAD *sec.UserAuthData)
}

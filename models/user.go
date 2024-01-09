package models

import (
    "strings"
	"cogged/log"
    sec "cogged/security"
)

type GraphUser struct {

	GraphBase		// embed

	Username 		*string 		    `json:"un,omitempty"`
	PasswordHash 	*string 		    `json:"ph,omitempty"`
	Data 			*string 		    `json:"us,omitempty"`
	InternalData 	*string 		    `json:"intd,omitempty"`
	Role 			*string 		    `json:"role,omitempty"`
	Nodes 			*[]*GraphNode       `json:"nodes,omitempty"`
	Shared 			*[]*GraphNode       `json:"shr,omitempty"`
}


func NewGraphUser(userUid string) *GraphUser {
    return &GraphUser{
        GraphBase: GraphBase{Uid: userUid},
    }
}


func (u *GraphUser) AuthzDataPack(key string) {
	ad := u.Uid + "."
	if u.Role != nil { ad += (*u.Role) }
	ad += "."
	u.AuthzData = sec.MessageAndMAC(ad, key)
}



func GraphUserFromUnpackedAD(adStr string) *GraphUser {
	if adStr != "" {
		parts := strings.Split(adStr,".")
		if len(parts) == 3 {
			uid := parts[0]
			role := parts[1]
			data := parts[2]
			n := NewGraphUser(uid)
            n.Role = &role
            n.Data = &data
			return n
		}
	}
	return nil
}


func GraphUserFromAD(packedAuthzData, key string) *GraphUser {
	// authzData is <b64data>.<hmac> string
	adStr := DecodeAndVerifyAD(packedAuthzData, key)
	return GraphUserFromUnpackedAD(adStr)
}

func (u *GraphUser) AuthzDataUnpack(uad sec.UserAuthData, permissionsRequired string) bool {
    user := GraphUserFromAD(u.AuthzData, uad.SecretKey)
    return user != nil
}


func AuthzDataUnpackUserADStringSlice(adSlice *[]string, uad sec.UserAuthData, permsRequired string) bool {
	if adSlice != nil && len(*adSlice) > 0 {
		for i, ads := range *adSlice {
			adStr := DecodeAndVerifyAD(ads, uad.SecretKey)
log.Debug("AuthzDataUnpackUserADStringSlice> adStr", adStr)
			if adStr != "" {
				(*adSlice)[i] = adStr
				continue
			}
			return false
		}
log.Debug("AuthzDataUnpackUserADStringSlice> return true")
		return true
	}
	return false
}


//AuthzDataUnpackUserSlice
func AuthzDataUnpackUserSlice(userSlice *[]*GraphUser, uad sec.UserAuthData, permsRequired string) bool {
	if userSlice != nil && len(*userSlice) > 0 {
		for _, n := range *userSlice {
			if n != nil {
				ads := (*n).AuthzData
				if ads != "" {
					tmpUser := GraphUserFromAD(ads, uad.SecretKey)
					if (tmpUser != nil) {
						continue
					}
				}
			}
			return false
		}
		return true
	}
	return false
}

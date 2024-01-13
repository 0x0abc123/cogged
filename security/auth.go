package security

import (
	"strings"
	"crypto/hmac"
	"crypto/sha256"	
)

const SYS_ROLE = "sys"

type UserAuthData struct {
	Uid			string
	Role		string
	Timestamp	string
	SecretKey	string
}


func (u *UserAuthData) IsAdmin() bool {
	return u.Role == SYS_ROLE
}


func MAC(message, key []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(message)
	return mac.Sum(nil)
}


func MessageAndMAC(message, key string) string {
	m64 := B64Encode([]byte(message))
	mac64 := B64Encode(MAC([]byte(m64), B64Decode(key)))
	return m64 + "." + mac64
}


func ConstructToken(uid, role, timestamp, key string) string {
	t := ""
	t = uid + "." + role + "." + timestamp
	return MessageAndMAC(t,key)
}


func IsValidMAC(message, messageMAC, key []byte) bool {
	expectedMAC := MAC(message, key)
	return hmac.Equal(messageMAC, expectedMAC)
}


func UserKeyFromMasterSecret(masterKey, uid, role string) string {
	b := append(B64Decode(masterKey), []byte(uid+"::"+role)...) 
	return B64Encode(SHA512Hash(b))
}


func UADFromToken(token, key string) *UserAuthData {
	tp := strings.Split(token,".")
	if len(tp) != 2 {
		return nil
	}
	t64 := tp[0]
	tmac64 := tp[1]
	if !IsValidMAC([]byte(t64), B64Decode(tmac64), B64Decode(key)) {
		return nil
	}

	up := strings.Split(string(B64Decode(t64)),".")
	if len(up) != 3 {
		return nil
	}

	return &UserAuthData{
		Uid: up[0],
		Role: up[1],
		Timestamp: up[2],
		SecretKey: UserKeyFromMasterSecret(key,up[0],up[1]),
	}
}
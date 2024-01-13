package api

import (
	"reflect"
	"encoding/json"
	"cogged/log"
	res "cogged/responses"
	sec "cogged/security"
)

func tryAuthzDataPack[T any](v *T, uad *sec.UserAuthData) bool {
    adInterface := reflect.TypeOf(new(res.AuthzDataPacker)).Elem()
	log.Debug("TryAuthzDataPack", adInterface, reflect.TypeOf(v), reflect.TypeOf(v).Implements(adInterface))
    if reflect.TypeOf(v).Implements(adInterface) {
		any(v).(res.AuthzDataPacker).AuthzDataPack(uad)
	}
	return true
}


func MarshalJSON[T any](responseStruct *T, uad *sec.UserAuthData) string {
	tryAuthzDataPack[T](responseStruct, uad)
	b, err := json.Marshal(*responseStruct)
	if err!= nil {
		return ""
	}
	return string(b)
}
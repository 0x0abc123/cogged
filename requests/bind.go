package requests

import (
	"reflect"
	"encoding/json"
	"cogged/log"
	sec "cogged/security"
)


type BindError struct {
	Info string
}


func (e *BindError) Error() string {
	return e.Info
}


type UnpackData struct {
	UAD *sec.UserAuthData
	RequiredPermissions string
}


func TryAuthzDataUnpack[T any](v *T, ud UnpackData) bool {
    adInterface := reflect.TypeOf(new(AuthzDataUnpacker)).Elem()
log.Debug("TryAuthzDataUnpack", adInterface, reflect.TypeOf(v), reflect.TypeOf(v).Implements(adInterface))
    if reflect.TypeOf(v).Implements(adInterface) {
log.Debug("TryAuthzDataUnpack", ud)
		uad := ud.UAD
		if uad == nil {uad = &sec.UserAuthData{}}
		return any(v).(AuthzDataUnpacker).AuthzDataUnpack(*uad, ud.RequiredPermissions)
	}
	return false
}


func Validate[T any](v *T) bool {
    validaterInterface := reflect.TypeOf(new(Validater)).Elem()
log.Debug("Validater", validaterInterface, reflect.TypeOf(v), reflect.TypeOf(v).Implements(validaterInterface))
    if reflect.TypeOf(v).Implements(validaterInterface) {
		return any(v).(Validater).Validate()
	}
	return false
}

func BindToRequest[T any](jsonString string, requestStruct *T, ud UnpackData) error {
log.Debug("BindToRequest", jsonString, ud)
	err := json.Unmarshal([]byte(jsonString), requestStruct)
	if err == nil && TryAuthzDataUnpack[T](requestStruct, ud) && Validate[T](requestStruct) {
		return nil
	}

	return &BindError{Info: "unpack and validate failed"}
}

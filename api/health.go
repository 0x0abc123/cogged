package api

import (
	"cogged/log"
	sec "cogged/security"
)

type HealthAPI struct {}

func (h *HealthAPI) HandleRequest(handlerKey, param, body string, uad *sec.UserAuthData) (string, error) {
	log.Debug("HealthAPI: HandleRequest", handlerKey, param, body)
	d := map[string]string{"status": "ok"}
	return MarshalJSON[map[string]string](&d, uad), nil
}
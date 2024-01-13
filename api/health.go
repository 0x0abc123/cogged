package api

import (
	sec "cogged/security"
)

type HealthAPI struct {}

func NewHealthAPI() *HealthAPI {
	return &HealthAPI{}
}

func (h *HealthAPI) HandleRequest(handlerKey, param, body string, uad *sec.UserAuthData) (string, error) {
	d := map[string]string{"status": "ok"}
	return MarshalJSON[map[string]string](&d, uad), nil
}
package api

import (
	sec "cogged/security"
)

type Handler interface {
	HandleRequest(handlerKey, param, body string, uad *sec.UserAuthData) (string, error)
}
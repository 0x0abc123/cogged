// Package api provides the HTTP handlers, one per route group (auth, admin, graph, user,
// health). Each group implements Handler and is dispatched by the server's ServeHTTP using
// a "METHOD endpoint" key. Handlers are thin glue over the services and models packages.
package api

import (
	sec "cogged/security"
)

type Handler interface {
	HandleRequest(handlerKey, param, body string, uad *sec.UserAuthData) (string, error)
}
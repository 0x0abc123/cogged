package api

type APIError struct {
	Info string
}

func (e *APIError) Error() string {
	return e.Info
}
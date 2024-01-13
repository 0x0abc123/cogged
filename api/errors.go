package api

type APIError struct {
	Info string
	StatusCode int
}

func (e APIError) Error() string {
	return e.Info
}
package responses

type TokenResponse struct {
	Token	string	`json:"token"`
	Expires	int		`json:"exp"`  // expires in N seconds
}
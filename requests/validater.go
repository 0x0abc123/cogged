// Package requests defines the inbound JSON request DTOs and their two behavioural seams:
// Validater (input validation) and AuthzDataUnpacker (verifying client-supplied AuthzData
// tokens and substituting the real UIDs before a request is acted on).
package requests

type Validater interface {
	Validate() bool 
}
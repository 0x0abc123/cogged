package requests

import (
	"cogged/log"
	cm "cogged/models"
	sec "cogged/security"
)

type ShareNodesRequest struct {

	Nodes 		*[]string	`json:"nodes,omitempty"`
	Users 		*[]string	`json:"users,omitempty"`
}


func (req *ShareNodesRequest) AuthzDataUnpack(uad sec.UserAuthData, permissionsRequired string) bool {
	log.Debug("ShareNodesRequest.AuthzDataUnpack", *req)
	return (cm.AuthzDataUnpackADStringSlice(req.Nodes, uad, permissionsRequired) &&
			cm.AuthzDataUnpackUserADStringSlice(req.Users, uad, ""))
}


func (req *ShareNodesRequest) Validate() bool {
	return true
}
package requests

import (
	"cogged/log"
	cm "cogged/models"
	sec "cogged/security"
)

type ShareNodesRequest struct {

	Nodes 		*[]string	`json:"nodes,omitempty"`
	Users 		*[]string	`json:"users,omitempty"`
	UnpackedNodes	*[]*cm.GraphNode
}


func (req *ShareNodesRequest) AuthzDataUnpack(uad sec.UserAuthData, permissionsRequired string) bool {
	log.Debug("ShareNodesRequest.AuthzDataUnpack", *req)
	req.UnpackedNodes = &[]*cm.GraphNode{}
	return (cm.AuthzDataUnpackADStringSlicePlusNodes(req.Nodes, req.UnpackedNodes, uad, permissionsRequired) &&
			cm.AuthzDataUnpackUserADStringSlice(req.Users, uad, ""))
}


func (req *ShareNodesRequest) Validate() bool {
	return true
}

package requests

import (
	"cogged/log"
	cm "cogged/models"
	sec "cogged/security"
)

type UpdateNodesRequest struct {
	Nodes	*[]*cm.GraphNode	`json:"nodes,omitempty"`
}


func (req *UpdateNodesRequest) AuthzDataUnpack(uad sec.UserAuthData, permissionsRequired string) bool {
log.Debug("UpdateNodesRequest.AuthzDataUnpack", uad, permissionsRequired)
	return cm.AuthzDataUnpackNodeSlice(req.Nodes, uad, permissionsRequired)
}


func (req *UpdateNodesRequest) Validate() bool {
	return true
}
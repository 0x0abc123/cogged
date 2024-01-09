package requests

import (
	cm "cogged/models"
	sec "cogged/security"
)

type UserNodeRequest struct {
	Node *cm.GraphNode	`json:"node,omitempty"`
}


// not applicable, as the request does not access an existing UID, just a $placeholder UID for a new node
func (req *UserNodeRequest) AuthzDataUnpack(uad sec.UserAuthData, permissionsRequired string) bool {
	return true
}


func (req *UserNodeRequest) Validate() bool {
	return (*req).Node != nil && CheckUidIsPlaceholder((*req).Node.Uid)
}
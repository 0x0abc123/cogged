package requests

import (
	"strings"
	sec "cogged/security"
	cm "cogged/models"
)

const TMP_UID_PREFIX = "$"

type CreateNodesRequest struct {
	Nodes	*[]*cm.GraphNode	`json:"nodes,omitempty"`
	ResetSgi	bool	`json:"reset_sgi"`
}


// there should be no AuthzData because there should be no actual UIDs of nodes or edges, just $placeholders
func (req *CreateNodesRequest) AuthzDataUnpack(uad sec.UserAuthData, permissionsRequired string) bool {
	return true
}


func CheckUidIsPlaceholder(uid string) bool {
	return strings.HasPrefix(uid, TMP_UID_PREFIX)
}


func CheckUidsArePlaceholders(nl *[]*cm.GraphNode) bool {
	if nl != nil {
		for _, n := range *nl {
			if CheckUidIsPlaceholder((*n).Uid) {
				if (*n).OutEdges != nil && len(*n.OutEdges) > 0 {
					cResult := CheckUidsArePlaceholders(n.OutEdges)
					if !cResult {
						return false
					}
				}
				continue
			}
			return false
		}
	}
	return true
}


func (req *CreateNodesRequest) Validate() bool {
	res := req.Nodes != nil && len(*req.Nodes) > 0 && CheckUidsArePlaceholders(req.Nodes)
	return res
}

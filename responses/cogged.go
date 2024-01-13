package responses

import (
	"time"
	cm "cogged/models"
	sec "cogged/security"
)

type CoggedResponse struct {
	
	ResultNodes		[]*cm.GraphNode				`json:"result_nodes,omitempty"`
	CreatedNodes	cm.NodePtrDictionary		`json:"created_nodes,omitempty"`
	CreatedUids		map[string]string			`json:"created_uids,omitempty"`
	ServerTime		*time.Time					`json:"timestamp"`	
	Error			string						`json:"error,omitempty"`
}

func (resp *CoggedResponse) AuthzDataPack(uad *sec.UserAuthData) {
	if resp.ResultNodes != nil {
		filteredNodes := []*cm.GraphNode{}
		for _, node := range resp.ResultNodes {
			owner := node.Owner
			if (owner!= nil && owner.Uid == uad.Uid) || uad.IsAdmin() || *node.PermRead {
				node.AuthzDataPack(uad)
				filteredNodes = append(filteredNodes, node)
			}
		}
		resp.ResultNodes = filteredNodes
	}

	if resp.CreatedNodes != nil {
		for _, node := range resp.CreatedNodes {
			node.AuthzDataPack(uad)
		}
	}

	// CreatedUids is only sent by PUT /admin/user so no need to do AuthzDataPack
}

func CoggedResponseFromNodes(nodes *[]*cm.GraphNode) *CoggedResponse {

	tnow := time.Now().UTC()
	cr := CoggedResponse{
		ServerTime: &tnow,
	}

	if nodes != nil {
		for _, node := range *nodes {
			node.DgraphType = nil
		}
		cr.ResultNodes = *nodes
	}
	return &cr
}

//func CoggedResponseFromNodesMap(m *map[string]*cm.GraphNode) *CoggedResponse {
func CoggedResponseFromNodesMap(m *cm.NodePtrDictionary) *CoggedResponse {

	tnow := time.Now().UTC()
	cr := CoggedResponse{
		CreatedNodes: *m,
		ServerTime: &tnow,
	}
	return &cr
}

func CoggedResponseFromUidsMap(m *map[string]string) *CoggedResponse {

	tnow := time.Now().UTC()
	cr := CoggedResponse{
		CreatedUids: *m,
		ServerTime: &tnow,
	}
	return &cr
}

func CoggedResponseFromError(e string) *CoggedResponse {

	tnow := time.Now().UTC()
	cr := CoggedResponse{
		Error: e,
		ServerTime: &tnow,
	}
	return &cr
}
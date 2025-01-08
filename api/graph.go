package api

import (
	svc "cogged/services"
	sec "cogged/security"
	req "cogged/requests"
	res "cogged/responses"
	cm "cogged/models"
)

type GraphAPI struct {
	Configuration	*svc.Config
	Database		*svc.DB
}


func NewGraphAPI(config *svc.Config, db *svc.DB) *GraphAPI {
	a := &GraphAPI{
		Configuration: config,
		Database: db,
	}
	return a
}


func (h *GraphAPI) HandleRequest(handlerKey, param, body string, uad *sec.UserAuthData) (string, error) {
	ud := req.UnpackData{ UAD: uad }
	uid := (*uad).Uid

	switch handlerKey {

		case "POST nodes":
			ud.RequiredPermissions = "r"
			r := &req.QueryRequest{}
			if berr := req.BindToRequest[req.QueryRequest](body, r, ud); berr != nil {
				return "", &APIError{Info: berr.Error(), StatusCode: 400}
			}
			cr := h.Database.QueryWithOptions(r, svc.NODENODE)
			return MarshalJSON[res.CoggedResponse](cr, uad), nil

		case "GET sharedwith":
			ud.RequiredPermissions = "s"
			tn := cm.AuthzDataUnpackADString(param, *ud.UAD, ud.RequiredPermissions)
			if tn == nil {
				return "", &APIError{Info: "cannot view users this node ID is shared with", StatusCode: 400}
			}
			existingNodeUid := (*tn).Uid

			cr,_ := h.Database.QueryUsersThatNodeIsSharedWith(existingNodeUid)
			return MarshalJSON[res.CoggedResponse](cr, uad), nil

		case "PATCH nodes":
			ud.RequiredPermissions = "w"
			r := &req.UpdateNodesRequest{}
			if berr := req.BindToRequest[req.UpdateNodesRequest](body, r, ud); berr != nil {
				return "", &APIError{Info: berr.Error(), StatusCode: 400}
			}
			cr,_ := h.Database.UpsertNodes(r.Nodes)
			return MarshalJSON[res.CoggedResponse](cr, uad), nil

		case "PUT nodes":
			ud.RequiredPermissions = "o"
			tn := cm.AuthzDataUnpackADString(param, *ud.UAD, ud.RequiredPermissions)
			if tn == nil {
				return "", &APIError{Info: "invalid create nodes parent ID", StatusCode: 400}
			}
			existingNodeUid := (*tn).Uid
			existingNodeSgi := (*tn).Sgi

			r := &req.CreateNodesRequest{}
			if berr := req.BindToRequest[req.CreateNodesRequest](body, r, ud); berr != nil {
				return "", &APIError{Info: berr.Error(), StatusCode: 400}
			}

			var sgiForNewNodes string
			if r.ResetSgi {
				sgiForNewNodes = sec.GenerateSgi()
			} else {
				sgiForNewNodes = *existingNodeSgi
			}

			newnodes := *r.Nodes
			newEdges := make(cm.NodePtrDictionary)
			nodesLinkedFromOtherNewNodes := make(map[string]bool)

			nodeOwnerUid := uid
			
			// do some further processing and validation of UIDs (Validate() has alreday checked whether UIDs are non-empty and start with $)
        	// and figure out if any of the new nodes form a subgraph
			for _, n := range newnodes {
				(*n).Owner = cm.NewGraphUser(nodeOwnerUid)
				(*n).Sgi = &sgiForNewNodes
				nOE := (*n).OutEdges
				if nOE != nil && len(*nOE) > 0 {
					for i, e := range *nOE {
						edgeUid := (*e).Uid
						if edgeUid == (*n).Uid {
							return "", &APIError{Info: "invalid update nodes request (self link disallowed)", StatusCode: 400}
						}
						// Only allow one level of depth for OutEdges, i.e. no multilevel nested edges
						(*nOE)[i] = cm.NewGraphNodeJustUID(edgeUid)
						nodesLinkedFromOtherNewNodes[edgeUid] = true
					}
				}
			} 

			atLeastOneNewNodeIsChildOfExistingNode := false

			for _, n := range newnodes {
				nUid := (*n).Uid
				if nodesLinkedFromOtherNewNodes[nUid] {
					continue
				}
				atLeastOneNewNodeIsChildOfExistingNode = true
				svc.StoreNodeOutgoingEdgeData(&newEdges, existingNodeUid, nUid)
			}

			if !atLeastOneNewNodeIsChildOfExistingNode {
				//return "", &APIError{Info: "at least one node must not have an inlink from another node in the new nodes list", StatusCode: 400}
				svc.StoreNodeOutgoingEdgeData(&newEdges, existingNodeUid, (*newnodes[0]).Uid)
			}

			for _, e := range newEdges {
				newnodes = append(newnodes, e)
			}

			cr,_ := h.Database.UpsertNodes(&newnodes)
			return MarshalJSON[res.CoggedResponse](cr, uad), nil

		case "PUT edges":
			//note: permissions are checked in EdgesRequest.AuthzDataUnpack()
			r := req.EdgesRequest{}
			if berr := req.BindToRequest[req.EdgesRequest](body, &r, ud); berr != nil {
				return "", &APIError{Info: berr.Error(), StatusCode: 400}
			}
			cr,_ := h.Database.AddNodeEdges(r.SubjectIds, r.IncomingIds, r.OutgoingIds) 
			return MarshalJSON[res.CoggedResponse](cr, uad), nil

		case "PATCH edges":
			//note: permissions are checked in EdgesRequest.AuthzDataUnpack()
			r := req.EdgesRequest{}
			if berr := req.BindToRequest[req.EdgesRequest](body, &r, ud); berr != nil {
				return "", &APIError{Info: berr.Error(), StatusCode: 400}
			}
			cr,_ := h.Database.RemoveNodeEdges(r.SubjectIds, r.IncomingIds, r.OutgoingIds) 
			return MarshalJSON[res.CoggedResponse](cr, uad), nil

	}

	return "", &APIError{Info: "not found", StatusCode: 404}
}

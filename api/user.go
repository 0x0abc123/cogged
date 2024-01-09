package api

import (
	"cogged/log"
	svc "cogged/services"
	sec "cogged/security"
	req "cogged/requests"
	res "cogged/responses"
	cm "cogged/models"
)

type UserAPI struct {
	Configuration	*svc.Config
	Database		*svc.DB
}

func (h *UserAPI) HandleRequest(handlerKey, param, body string, uad *sec.UserAuthData) (string, error) {
	log.Debug("UserAPI: HandleRequest", handlerKey, param, body)

	ud := req.UnpackData{ UAD: uad }

	//sk := (*uad).SecretKey
	uid := (*uad).Uid
	role := (*uad).Role

	switch handlerKey {

		case "PUT node":
			r := req.UserNodeRequest{}
			if berr := req.BindToRequest[req.UserNodeRequest](body, &r, ud); berr != nil {
				return "", &APIError{Info: berr.Error()}
			}
			cr,_ := h.Database.UpsertUserNode(r.Node, uid)
			return MarshalJSON[res.CoggedResponse](cr, uad), nil

		case "POST nodes":
			// this should normally require "r" permission (ud.RequiredPermissions = "r")
			// if a node has been shared with another user, it is implied that they can read without explicit PermRead set
			var edgeType svc.EdgeType
			switch param {
			case "shared":
				edgeType = svc.USERSHARE
			case "own":
				edgeType = svc.USERNODE
			default:
				return "", &APIError{Info: "invalid edge type"}
			}
			r := req.QueryRequest{}
			if berr := req.BindToRequest[req.QueryRequest](body, &r, ud); berr != nil {
				return "", &APIError{Info: berr.Error()}
			}
			r.RootIDs = []string{uid}
			r.RootQuery = nil
			r.Depth = 1
			cr := h.Database.QueryWithOptions(&r, edgeType)
			return MarshalJSON[res.CoggedResponse](cr, uad), nil

		case "PUT share":
			ud.RequiredPermissions = "s"
			r := req.ShareNodesRequest{}
			if berr := req.BindToRequest[req.ShareNodesRequest](body, &r, ud); berr != nil {
				return "", &APIError{Info: berr.Error()}
			}

			usersToShareWith := []string{}

			for _, ads := range *r.Users {
				u := cm.GraphUserFromUnpackedAD(ads)
				if *u.Role != sec.SYS_ROLE {
					usersToShareWith = append(usersToShareWith, u.Uid)
				} else {
					return "", &APIError{Info: "user not found"}
				}
			} 

			cr,_ := h.Database.UpdateUserShareEdges(r.Nodes, &usersToShareWith, svc.ADD) 
			return MarshalJSON[res.CoggedResponse](cr, uad), nil

		case "DELETE share":
			ud.RequiredPermissions = "s"
			r := req.ShareNodesRequest{}
			if berr := req.BindToRequest[req.ShareNodesRequest](body, &r, ud); berr != nil {
				return "", &APIError{Info: berr.Error()}
			}

			usersToShareWith := []string{}

			for _, ads := range *r.Users {
				u := cm.GraphUserFromUnpackedAD(ads)
				if *u.Role != sec.SYS_ROLE {
					usersToShareWith = append(usersToShareWith, u.Uid)
				} else {
					return "", &APIError{Info: "user not found"}
				}
			} 

			cr,_ := h.Database.UpdateUserShareEdges(r.Nodes, &usersToShareWith, svc.DELETE) 
			return MarshalJSON[res.CoggedResponse](cr, uad), nil

		case "GET name":
			ur,_ := h.Database.QueryUser(param)
			u := (*ur).User
			if u != nil && canAccessUser(role, u) {
				(*ur).User = ReturnUserDTO(u)
				return MarshalJSON[res.UserResponse](ur, uad), nil
			}
			return "", &APIError{Info: (*ur).Error}

		case "GET uid":
			if !svc.ValidateUid(param) {
				return "", &APIError{Info: "invalid uid"}
			}			
			ur,_ := h.Database.QueryUserByUid(param, false)
			u := (*ur).User
			if u != nil && canAccessUser(role, u) {
				(*ur).User = ReturnUserDTO(u)
				return MarshalJSON[res.UserResponse](ur, uad), nil
			}
log.Debug("UserAPI:GET uid", u, param, body)
			return "", &APIError{Info: (*ur).Error}

	}

	return "", &APIError{Info: "not found"}
}


func canAccessUser(role string, dbuser *cm.GraphUser) bool {
	return dbuser != nil && (role == sec.SYS_ROLE || *dbuser.Role != sec.SYS_ROLE)
}


func ReturnUserDTO(dbuser *cm.GraphUser) *cm.GraphUser {
	u := cm.NewGraphUser("")
	u.Uid = dbuser.Uid
	u.Username = dbuser.Username
	u.Role = dbuser.Role
	u.Data = dbuser.Data
	return u
}
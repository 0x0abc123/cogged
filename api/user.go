package api

import (
	svc "cogged/services"
	sec "cogged/security"
	req "cogged/requests"
	res "cogged/responses"
	cm "cogged/models"
	state "cogged/state"
)

type UserAPI struct {
	Configuration	*svc.Config
	Database		*svc.DB
}


func NewUserAPI(config *svc.Config, db *svc.DB) *UserAPI {
	a := &UserAPI{
		Configuration: config,
		Database: db,
	}
	return a
}

func AllowListSharedSgis(uid string, nl []*cm.GraphNode) {
	UpdateAllowListSharedSgis(true, uid, nl)
}

func RevokeSharedSgis(uid string, nl []*cm.GraphNode) {
	UpdateAllowListSharedSgis(false, uid, nl)
}

func UpdateAllowListSharedSgis(allow bool, uid string, nl []*cm.GraphNode) {
	sgiSet := make(map[string]bool)

	for _, node := range nl {
		owner := node.Owner
		if (owner!= nil && owner.Uid == uid) {
			continue
		}
		if *node.PermRead && node.Sgi != nil {
			sgiSet[*node.Sgi] = true
		}
	}

	sgiList := ""
	for key, _ := range sgiSet {
		sgiList += key + ","
	}

	if sgiList != "" {
		if allow {
			state.UsmUserAllowlistSgi(uid, sgiList)
		} else {
			state.UsmUserRevokeSgi(uid, sgiList)
		}
	}
}

func (h *UserAPI) HandleRequest(handlerKey, param, body string, uad *sec.UserAuthData) (string, error) {
	ud := req.UnpackData{ UAD: uad }
	uid := (*uad).Uid
	role := (*uad).Role

	switch handlerKey {

		case "PUT node":
			r := req.UserNodeRequest{}
			if berr := req.BindToRequest[req.UserNodeRequest](body, &r, ud); berr != nil {
				return "", &APIError{Info: berr.Error(), StatusCode: 400}
			}
			ts := sec.GenerateSgi()
			r.Node.Sgi = &ts
			cr,_ := h.Database.UpsertUserNode(r.Node, uid)
			return MarshalJSON[res.CoggedResponse](cr, uad), nil

		case "POST nodes":
			// this should normally require "r" permission (ud.RequiredPermissions = "r")
			// but if a node has been shared with another user, it is implied that they can read without explicit PermRead set
			var edgeType svc.EdgeType
			switch param {
			case "shared":
				edgeType = svc.USERSHARE
			case "own":
				edgeType = svc.USERNODE
			default:
				return "", &APIError{Info: "invalid edge type", StatusCode: 400}
			}
			r := req.QueryRequest{}
			if berr := req.BindToRequest[req.QueryRequest](body, &r, ud); berr != nil {
				return "", &APIError{Info: berr.Error(), StatusCode: 400}
			}
			r.RootIDs = []string{uid}
			r.RootQuery = nil
			r.Depth = 1
			cr := h.Database.QueryWithOptions(&r, edgeType)
			if param == "shared" && !uad.IsAdmin() {
				AllowListSharedSgis(uad.Uid, cr.ResultNodes)
			}
			return MarshalJSON[res.CoggedResponse](cr, uad), nil

		case "PUT share":
			ud.RequiredPermissions = "s"
			r := req.ShareNodesRequest{}
			if berr := req.BindToRequest[req.ShareNodesRequest](body, &r, ud); berr != nil {
				return "", &APIError{Info: berr.Error(), StatusCode: 400}
			}

			usersToShareWith := []string{}

			for _, ads := range *r.Users {
				u := cm.GraphUserFromUnpackedAD(ads)
				if *u.Role != sec.SYS_ROLE {
					usersToShareWith = append(usersToShareWith, u.Uid)
				} else {
					return "", &APIError{Info: "user not found", StatusCode: 404}
				}
			} 

			cr,_ := h.Database.UpdateUserShareEdges(r.Nodes, &usersToShareWith, svc.ADD)
			for _,tu := range usersToShareWith {
				AllowListSharedSgis(tu, *r.UnpackedNodes)
			}
			return MarshalJSON[res.CoggedResponse](cr, uad), nil

		case "PATCH share":
			ud.RequiredPermissions = "s"
			r := req.ShareNodesRequest{}
			if berr := req.BindToRequest[req.ShareNodesRequest](body, &r, ud); berr != nil {
				return "", &APIError{Info: berr.Error(), StatusCode: 400}
			}

			usersToShareWith := []string{}

			for _, ads := range *r.Users {
				u := cm.GraphUserFromUnpackedAD(ads)
				if *u.Role != sec.SYS_ROLE {
					usersToShareWith = append(usersToShareWith, u.Uid)
				} else {
					return "", &APIError{Info: "user not found", StatusCode: 400}
				}
			} 

			cr,_ := h.Database.UpdateUserShareEdges(r.Nodes, &usersToShareWith, svc.DELETE) 
			for _,tu := range usersToShareWith {
				RevokeSharedSgis(tu, *r.UnpackedNodes)
			}
			return MarshalJSON[res.CoggedResponse](cr, uad), nil

		case "GET name":
			ur,_ := h.Database.QueryUser(param)
			u := (*ur).User
			if u != nil && canAccessUser(role, u) {
				(*ur).User = ReturnUserDTO(u)
				return MarshalJSON[res.UserResponse](ur, uad), nil
			}
			return "", &APIError{Info: (*ur).Error, StatusCode: 404}

		case "GET uid":
			if !svc.ValidateUid(param) {
				return "", &APIError{Info: "invalid uid", StatusCode: 400}
			}			
			ur,_ := h.Database.QueryUserByUid(param, false)
			u := (*ur).User
			if u != nil && canAccessUser(role, u) {
				(*ur).User = ReturnUserDTO(u)
				return MarshalJSON[res.UserResponse](ur, uad), nil
			}
			return "", &APIError{Info: (*ur).Error, StatusCode: 404}

	}

	return "", &APIError{Info: "not found", StatusCode: 404}
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

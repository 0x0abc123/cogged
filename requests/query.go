package requests

import (
	"cogged/log"
	cm "cogged/models"
	sec "cogged/security"
)


type QueryRequestClause struct {

	And []QueryRequestClause	`json:"and,omitempty"`
	Or []QueryRequestClause		`json:"or,omitempty"`
	Field string				`json:"field,omitempty"`
	Op string					`json:"op,omitempty"`
	Val string					`json:"val,omitempty"`
}


type QueryRequest struct {

	RootIDs		[]string	`json:"root_ids,omitempty"`
	RootQuery	*QueryRequestClause	`json:"root_query,omitempty"`
	Depth		uint		`json:"depth"`
	Filters		*QueryRequestClause	`json:"filters,omitempty"`
	Select		[]string	`json:"select,omitempty"`
}


func (req *QueryRequest) AuthzDataUnpack(uad sec.UserAuthData, permissionsRequired string) bool {
	log.Debug("QueryRes AuthzDataUnpack", req)
	// only system role can supply a root query:
	if uad.Role != sec.SYS_ROLE && req.RootQuery != nil {
		return false
	}
	// allowe empty root IDs (for system users running a rootQuery or POST /user/nodes/shared|own)
	if len(req.RootIDs) < 1 {
		return true
	}
	return cm.AuthzDataUnpackADStringSlice(&req.RootIDs, uad, permissionsRequired)
}


func (req *QueryRequest) Validate() bool {
	return true
}
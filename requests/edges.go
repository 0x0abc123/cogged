package requests

import (
	cm "cogged/models"
	sec "cogged/security"
)

type EdgesRequest struct {
	SubjectIds  *[]string `json:"subject_ids,omitempty"`
	IncomingIds *[]string `json:"incoming_ids,omitempty"`
	OutgoingIds *[]string `json:"outgoing_ids,omitempty"`
}

// authzOptionalIDs authorizes an optional id list. An absent (nil/empty) list has
// nothing to check and passes; a present list must fully authorize with `perms`.
func authzOptionalIDs(ids *[]string, uad sec.UserAuthData, perms string) bool {
	if ids == nil || len(*ids) == 0 {
		return true
	}
	return cm.AuthzDataUnpackADStringSlice(ids, uad, perms)
}

func (req *EdgesRequest) AuthzDataUnpack(uad sec.UserAuthData, permissionsRequired string) bool {
	permissionsForSubjectNodes := ""
	if req.IncomingIds != nil && len(*req.IncomingIds) > 0 {
		permissionsForSubjectNodes += "i"
	}
	if req.OutgoingIds != nil && len(*req.OutgoingIds) > 0 {
		permissionsForSubjectNodes += "o"
	}
	// SubjectIds are required; incoming/outgoing are each optional, and only the
	// lists that are actually supplied are authorized (so a single-direction edge
	// request is allowed instead of failing closed on the absent list).
	return cm.AuthzDataUnpackADStringSlice(req.SubjectIds, uad, permissionsForSubjectNodes) &&
		authzOptionalIDs(req.IncomingIds, uad, "o") &&
		authzOptionalIDs(req.OutgoingIds, uad, "i")
}

func (req *EdgesRequest) Validate() bool {
	if req.SubjectIds == nil || len(*req.SubjectIds) == 0 {
		return false
	}
	hasIn := req.IncomingIds != nil && len(*req.IncomingIds) > 0
	hasOut := req.OutgoingIds != nil && len(*req.OutgoingIds) > 0
	// at least one edge direction must be supplied, else the request is a no-op
	return hasIn || hasOut
}

/*
# create edges from 0x123 -> 0x206 and 0x206 -> 0x345
{
  "subjectIds": ["0x206"],
  "incomingIds": ["0x123"],
  "outgoingIds": ["0x345"]
}

*/

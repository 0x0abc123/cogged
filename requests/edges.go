package requests

import (
	cm "cogged/models"
	sec "cogged/security"
)

type EdgesRequest struct {

	SubjectIds 		*[]string	`json:"subject_ids,omitempty"`
	IncomingIds 	*[]string	`json:"incoming_ids,omitempty"`
	OutgoingIds 	*[]string	`json:"outgoing_ids,omitempty"`
}


func (req *EdgesRequest) AuthzDataUnpack(uad sec.UserAuthData, permissionsRequired string) bool {
	permissionsForSubjectNodes := ""
	if req.IncomingIds != nil && len(*req.IncomingIds) > 0 {
		permissionsForSubjectNodes += "i"
	}
	if req.OutgoingIds != nil && len(*req.OutgoingIds) > 0 {
		permissionsForSubjectNodes += "o"
	}
	return (cm.AuthzDataUnpackADStringSlice(req.SubjectIds, uad, permissionsForSubjectNodes) &&
			cm.AuthzDataUnpackADStringSlice(req.IncomingIds, uad, "o") &&
			cm.AuthzDataUnpackADStringSlice(req.OutgoingIds, uad, "i"))	
}


func (req *EdgesRequest) Validate() bool {
	return true
}

/*
# create edges from 0x123 -> 0x206 and 0x206 -> 0x345
{
  "subjectIds": ["0x206"],
  "incomingIds": ["0x123"],
  "outgoingIds": ["0x345"]
}

*/
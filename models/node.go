package models

import (
	"time"
	"strings"
	sec "cogged/security"
)

type NodePtrDictionary map[string]*GraphNode

type GraphNode struct {

	GraphBase		// embed

    OutEdges 		*[]*GraphNode `json:"e,omitempty"`
	Owner			*GraphUser	`json:"own,omitempty"`
	PermRead		*bool		`json:"r,omitempty"`
	PermWrite		*bool		`json:"w,omitempty"`
	PermOutEdge		*bool		`json:"o,omitempty"`
	PermInEdge		*bool		`json:"i,omitempty"`
	PermDelete		*bool		`json:"d,omitempty"`
	PermShare		*bool		`json:"s,omitempty"`
	Id				*string		`json:"id,omitempty"`
	Type			*string		`json:"ty,omitempty"`
	PrivateData		*string		`json:"p,omitempty"`
	String1			*string		`json:"s1,omitempty"`
	String2			*string		`json:"s2,omitempty"`
	String3			*string		`json:"s3,omitempty"`
	String4			*string		`json:"s4,omitempty"`
	Blob			*string		`json:"b,omitempty"`
	Num1			*float64	`json:"n1,omitempty"`
	Num2			*float64	`json:"n2,omitempty"`
	TimeCreated		*time.Time	`json:"c,omitempty"`	
	TimeModified	*time.Time	`json:"m,omitempty"`	
	Time1			*time.Time	`json:"t1,omitempty"`	
	Time2			*time.Time	`json:"t2,omitempty"`	
	Location		*Geoloc		`json:"g,omitempty"`

	//TODO: g: geo .
    //{"type":"Point","coordinates":[2.3508,48.8567]}}
    //{"type":"Polygon","coordinates":[[[2.3508,48.8567],[2.3509,48.8567],[2.3509,48.8568],[2.3508,48.8567]]] }}

}


func DecodeAndVerifyAD(adAndMAC, key string) string {
	parts := strings.Split(adAndMAC, ".")
	if len(parts) == 2 {
		if sec.IsValidMAC([]byte(parts[0]), sec.B64Decode(parts[1]), []byte(sec.B64Decode(key))) {
			b := sec.B64Decode(parts[0])
			if len(b) > 0 {
				return string(b)
			}
		}
	}
	return ""
}


func (n *GraphNode) ConvertNullBoolFieldsToFalse() {
	b := false
	if n.PermRead == nil { n.PermRead = &b }
	if n.PermWrite == nil { n.PermWrite = &b }
	if n.PermOutEdge == nil { n.PermOutEdge = &b }
	if n.PermInEdge == nil { n.PermInEdge = &b }
	if n.PermDelete == nil { n.PermDelete = &b }
	if n.PermShare == nil { n.PermShare = &b }
}


func GraphNodeFromUnpackedAD(adStr string) *GraphNode {
	if adStr != "" {
		parts := strings.Split(adStr,".")
		if len(parts) == 3 {
			uid := parts[0]
			own := parts[1]
			perms := parts[2]
			n := NewGraphNodeJustUID(uid)
			n.Owner = &GraphUser{GraphBase: GraphBase{Uid: own}}
			t := true
			if strings.Contains(perms,"r") { n.PermRead = &t }
			if strings.Contains(perms,"w") { n.PermWrite = &t }
			if strings.Contains(perms,"o") { n.PermOutEdge = &t }
			if strings.Contains(perms,"i") { n.PermInEdge = &t }
			if strings.Contains(perms,"d") { n.PermDelete = &t }
			if strings.Contains(perms,"s") { n.PermShare = &t }
			return n
		}
	}
	return nil
}


func GraphNodeFromAD(packedAuthzData, key string) *GraphNode {
	// authzData is <b64data>.<hmac> string
	adStr := DecodeAndVerifyAD(packedAuthzData, key)
	return GraphNodeFromUnpackedAD(adStr)
}


func AuthzDataUnpackADString(ads string, uad sec.UserAuthData, permsRequired string) *GraphNode {
	tmpNode := GraphNodeFromAD(ads, uad.SecretKey)
	if tmpNode != nil {
		if (uad.Uid == (*tmpNode).Owner.Uid || uad.Role == sec.SYS_ROLE || tmpNode.HasRequiredPermissions(permsRequired)) {
			return tmpNode
		}
	}
	return nil
}


func AuthzDataUnpackADStringSlice(adSlice *[]string, uad sec.UserAuthData, permsRequired string) bool {
	if adSlice != nil && len(*adSlice) > 0 {
		for i, ads := range *adSlice {
			tmpNode := AuthzDataUnpackADString(ads, uad, permsRequired)
			if tmpNode != nil {
				(*adSlice)[i] = (*tmpNode).Uid
				continue
			}
			return false
		}
		return true
	}
	return false
}


func AuthzDataUnpackNodeSlice(nodeSlice *[]*GraphNode, uad sec.UserAuthData, permsRequired string) bool {
	if nodeSlice != nil && len(*nodeSlice) > 0 {
		for _, n := range *nodeSlice {
			if n != nil {
				ads := (*n).AuthzData
				if ads != "" {
					tmpNode := GraphNodeFromAD(ads, uad.SecretKey)
					if (tmpNode != nil && 
						AuthzFieldsAreEqual(n, tmpNode) && 
					    (uad.Uid == (*tmpNode).Owner.Uid || 
						 uad.Role == sec.SYS_ROLE || 
						 tmpNode.HasRequiredPermissions(permsRequired))) {
						continue
					}
				}
			}
			return false
		}
		return true
	}
	return false
}


func NewGraphNodeJustOwnerAndPerms(origNode *GraphNode) *GraphNode {
	if origNode == nil {
		return nil
	}
	newNode := &GraphNode{
		GraphBase: GraphBase{Uid: origNode.Uid},
	}
	if origNode.Owner != nil {
		newNode.Owner = origNode.Owner
	}
	newNode.PermRead = origNode.PermRead
	newNode.PermWrite = origNode.PermWrite
	newNode.PermOutEdge = origNode.PermOutEdge
	newNode.PermInEdge = origNode.PermInEdge
	newNode.PermDelete = origNode.PermDelete
	newNode.PermShare = origNode.PermShare

	return newNode
}


func NewGraphNodeJustUID(uid string) *GraphNode {
	return &GraphNode{
		GraphBase: GraphBase{Uid: uid},
	}
}


func NewGraphNodeEdge(parentUID string, childUID string) *GraphNode {
	return &GraphNode{
		GraphBase: GraphBase{Uid: parentUID},
		OutEdges: &[]*GraphNode{NewGraphNodeJustUID(childUID)},
	}
}


func (n *GraphNode) AuthzDataPack(key string) {
	ad := n.Uid + "."

	if n.Owner != nil { ad += (*n.Owner).Uid }

	ad += "."

	n.ConvertNullBoolFieldsToFalse()

	if *n.PermRead { ad += "r" }
	if *n.PermWrite { ad += "w" }
	if *n.PermOutEdge { ad += "o" }
	if *n.PermInEdge { ad += "i" }
	if *n.PermDelete { ad += "d" }
	if *n.PermShare { ad += "s" }

	n.AuthzData = sec.MessageAndMAC(ad, key)

	if n.OutEdges != nil {
		for _, e := range *n.OutEdges {
			e.AuthzDataPack(key)
		}
	}
	return
}


func (n *GraphNode) HasRequiredPermissions(rp string) bool {
	if rp == "" {
		return true
	}

	permissionsMet := true

	n.ConvertNullBoolFieldsToFalse()

	for _, c := range rp {
		switch c {
		case 'r':
			permissionsMet = permissionsMet && *n.PermRead
		case 'w':
			permissionsMet = permissionsMet && *n.PermWrite
		case 'o':
			permissionsMet = permissionsMet && *n.PermOutEdge
		case 'i':
			permissionsMet = permissionsMet && *n.PermInEdge
		case 'd':
			permissionsMet = permissionsMet && *n.PermDelete
		case 's':
			permissionsMet = permissionsMet && *n.PermShare
		}
		if !permissionsMet {
			break
		}
	}
	return permissionsMet
}


func AuthzFieldsAreEqual(n1, n2 *GraphNode) bool {
	if n1 != nil && n2 != nil {
		n1.ConvertNullBoolFieldsToFalse()
		n2.ConvertNullBoolFieldsToFalse()
		return (
			n1.Uid == n2.Uid &&
			n1.Owner != nil &&
			n2.Owner != nil &&
			(*n1.Owner).Uid == (*n2.Owner).Uid &&
			*n1.PermRead == *n2.PermRead &&
			*n1.PermWrite == *n2.PermWrite &&
			*n1.PermOutEdge == *n2.PermOutEdge &&
			*n1.PermInEdge == *n2.PermInEdge &&
			*n1.PermDelete == *n2.PermDelete &&
			*n1.PermShare == *n2.PermShare )
	}
	return false
}

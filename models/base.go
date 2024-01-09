package models

type GraphBaser interface {
	GetUid() string
	SetUid(uid string)
}

type GraphBase struct {
	
	Uid			string		`json:"uid"`
	DgraphType	[]string	`json:"dgraph.type,omitempty"`
	AuthzData	string		`json:"ad,omitempty"`
}

func (g *GraphBase) GetUid() string {
	return g.Uid
}

func (g *GraphBase) SetUid(uid string) {
	g.Uid = uid
}

func (g *GraphBase) GetAuthzData() string {
	return g.AuthzData
}

func (g *GraphBase) SetAuthzData(ad string) {
	g.AuthzData = ad
}
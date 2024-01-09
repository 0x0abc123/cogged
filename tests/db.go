package main

// go run tests/db.go -h 10.20.0.7 -p 9080

import (
	"fmt"
	"flag"
	"strings"
	"encoding/json"
	cm "cogged/models"
	svc "cogged/services"
	sec "cogged/security"
	req "cogged/requests"
)

const (
	TYPE_INBOX = "inbox"
	TYPE_FOLDER = "folder"
	TYPE_MESSAGE = "message"
	TYPE_OUTBOX = "outbox"
)

func dump(i interface{}) string {
	pb, _ := json.Marshal(i)
	return string(pb)
}

func pm(msg string) {
	h40 := strings.Repeat("#", 40)
	fmt.Printf("\n%s\n%s\n%s\n",h40,msg,h40)
}

func pr(res interface{}, err error) {
	fmt.Printf("RespJson:\n%s\nErr:\n%s\n",dump(res),err)
}

func main() {

	// CLI flags
	dropDB := flag.Bool("drop", false, "Drop the Dgraph database before running tests") 

	var flagDgraphHost string
	flag.StringVar(&flagDgraphHost, "h", "localhost", "URL for Dgraph host eg. 10.1.2.3")

	var flagDgraphPort string
	flag.StringVar(&flagDgraphPort, "p", "9080", "URL for Dgraph port eg. 9080")

	flag.Parse()

	// create DB struct and Dgraph client
	conf := make(svc.Config)
	conf["db.host"] = flagDgraphHost
	conf["db.port"] = flagDgraphPort

	db := svc.NewDB(&conf)
	fmt.Println(*db)

	if *dropDB {
		fmt.Printf("WARNING: THIS WILL CLEAR THE DGRAPH DATABASE ON %s:%s\nType 'dropall' to continue with clearing: ",flagDgraphHost,flagDgraphPort)
		var input string
		fmt.Scanln(&input)
		if input == "dropall" {
			fmt.Println("clearing database...")
			if err := db.DropAll(); err != nil {
				fmt.Println(err)
				return
			}
			fmt.Println("database cleared, run again to commence tests.")
			return
		}
		fmt.Println("drop database cancelled, goodbye")
		return
	}

	// random string for unique values for this test run
	b,_ := sec.GenerateRandomBytes(5)
	randdata := fmt.Sprintf("%x",b)

	// add user
	pm("add user1")
	u1name := "user1_"+randdata
	u1pass := "u1pass"
	u1data := "u1data"
	u1intd := "u1intd"
	u1role := "u1role"
	u1 := &cm.GraphUser{
		GraphBase: cm.GraphBase{Uid: "u1"},
		Username: &u1name,
		PasswordHash: &u1pass,
		Data:  &u1data,
		InternalData: &u1intd,
		Role: &u1role,
	}
	upU1slice := make([]*cm.GraphUser,1)
	upU1slice[0] = u1
	res1, err1 := db.UpsertUsers(&upU1slice)
	pr(res1,err1)

	// query user by username
	pm("query user1 by username")
	res2, err2 := db.QueryUser(u1name)
	pr(res2,err2)

	u1uid := res2.User.Uid
	u1user := &cm.GraphUser{
		GraphBase: cm.GraphBase{Uid: u1uid},
	}

	/*
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
	*/

	// add user node
	//func (db *DB) UpsertUserNode(node *cm.GraphNode, userUid string) (*res.CoggedResponse, error) {
	pm("add usernode(inbox) for user1")
	u1_inbox_id := "user1_"+randdata+"_inbox"
	u1_inbox_ty := TYPE_INBOX
	u1_inbox_p := "privdata for user 1 inbox"
	u1_inbox_s1 := "test[a]{a}<v>"
	u1_inbox_n := 100.0
	u1_inbox := &cm.GraphNode{
		GraphBase: cm.GraphBase{Uid: "u1_inbox"},
		Id: &u1_inbox_id,
		Type: &u1_inbox_ty,
		PrivateData: &u1_inbox_p,
		String1: &u1_inbox_s1,
		Num1: &u1_inbox_n,
	}
	res3, err3 := db.UpsertUserNode(u1_inbox, u1uid)
	pr(res3,err3)

	// add user node2
	//func (db *DB) UpsertUserNode(node *cm.GraphNode, userUid string) (*res.CoggedResponse, error) {
	pm("add 2nd usernode (outbox) for user1")
	u1_outbox_id := "user1_"+randdata+"_outbox"
	u1_outbox_ty := TYPE_OUTBOX
	u1_outbox_p := "privdata for user1 outbox"
	u1_outbox_s1 := "test second user node"
	u1_outbox_n := 123.4567
	u1_outbox := &cm.GraphNode{
		GraphBase: cm.GraphBase{Uid: "u1_outbox"},
		//Owner: u1user,
		Id: &u1_outbox_id,
		Type: &u1_outbox_ty,
		PrivateData: &u1_outbox_p,
		String1: &u1_outbox_s1,
		Num1: &u1_outbox_n,
	}
	res4, err4 := db.UpsertUserNode(u1_outbox, u1uid)
	pr(res4,err4)

	/*
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

	*/
	// get user nodes
	//func (d *DB) QueryWithOptions(q *req.QueryRequest, et EdgeType) *res.CoggedResponse {
	pm("query all usernodes for user1")
	query5 := &req.QueryRequest{
		RootIDs: []string{u1uid}, 
		Depth: uint(1), 
		Select: []string{"id","ty","s1","c","m"},
	}
	res5 := db.QueryWithOptions(query5, svc.USERNODE)
	pr(res5,nil)
	
	/*
	type CoggedResponse struct {

	ResultNodes		[]*cm.GraphNode				`json:"result_nodes,omitempty"`
	//CreatedNodes	map[string]*cm.GraphNode	`json:"created_nodes,omitempty"`
	CreatedNodes	cm.NodePtrDictionary		`json:"created_nodes,omitempty"`
	CreatedUids		map[string]string			`json:"created_uids,omitempty"`
	ServerTime		*time.Time					`json:"timestamp"`	
	Error			string						`json:"error,omitempty"`
	}
	*/

	// add small subgraph of nodes under inbox
	//func (db *DB) UpsertNodes(nodeList *[]*cm.GraphNode) (*res.CoggedResponse, error) {
	u1_inbox_uid := res3.CreatedNodes["new"].Uid

	pm("add small subgraph of nodes (folder, messages) under inbox")

	// orders -> message 1
	u1_orders_msg1_tmpuid := "u1_orders_msg1"
	u1_orders_msg1_id := "user1_"+randdata+"_orders_msg1"
	u1_orders_msg1_ty := TYPE_MESSAGE
	u1_orders_msg1_p := "privdata for msg1"
	u1_orders_msg1_s1 := "test subgraph"

	u1_orders_msg1 := cm.NewGraphNodeJustUID(u1_orders_msg1_tmpuid)
	u1_orders_msg1.Owner = u1user
	u1_orders_msg1.Id = &u1_orders_msg1_id
	u1_orders_msg1.Type = &u1_orders_msg1_ty
	u1_orders_msg1.PrivateData = &u1_orders_msg1_p
	u1_orders_msg1.String1 = &u1_orders_msg1_s1

	// orders -> message 2
	u1_orders_msg2_tmpuid := "u1_orders_msg2"
	u1_orders_msg2_id := "user1_"+randdata+"_orders_msg2"
	u1_orders_msg2_ty := TYPE_MESSAGE
	u1_orders_msg2_p := "privdata for msg2"
	u1_orders_msg2_s1 := "test subgraph"

	u1_orders_msg2 := cm.NewGraphNodeJustUID(u1_orders_msg2_tmpuid)
	u1_orders_msg2.Owner = u1user
	u1_orders_msg2.Id = &u1_orders_msg2_id
	u1_orders_msg2.Type = &u1_orders_msg2_ty
	u1_orders_msg2.PrivateData = &u1_orders_msg2_p
	u1_orders_msg2.String1 = &u1_orders_msg2_s1

	// orders folder
	u1_orders_tmpuid := "u1_orders"
	u1_orders_id := "user1_"+randdata+"_orders"
	u1_orders_ty := TYPE_FOLDER
	u1_orders_p := "privdata"
	u1_orders_s1 := "test[a]{a}<v>"

	u1_orders := cm.NewGraphNodeJustUID(u1_orders_tmpuid)
	u1_orders.Owner = u1user
	u1_orders.Id = &u1_orders_id
	u1_orders.Type = &u1_orders_ty
	u1_orders.PrivateData = &u1_orders_p
	u1_orders.String1 = &u1_orders_s1
	u1_orders.OutEdges = &[]*cm.GraphNode{
		cm.NewGraphNodeJustUID(u1_orders_msg1_tmpuid),
		cm.NewGraphNodeJustUID(u1_orders_msg2_tmpuid),
	}

	// edge from inbox -> orders
	edge_u1inbox_orders := cm.NewGraphNodeEdge(u1_inbox_uid, u1_orders_tmpuid)
	
	var upsertNodeList *[]*cm.GraphNode
	upsertNodeList = &[]*cm.GraphNode{
		u1_orders, 
		u1_orders_msg1,
		u1_orders_msg2,
		edge_u1inbox_orders,
	}
	res6, err6 := db.UpsertNodes(upsertNodeList)
	pr(res6,err6)

	// query nodes
	pm("query subgraph user1 inbox")
	query7 := &req.QueryRequest{
		RootIDs: []string{u1_inbox_uid}, 
		Depth: uint(20), 
		Select: []string{"e","id","ty","s1","c","m"},
	}
	res7 := db.QueryWithOptions(query7, svc.NODENODE)
	pr(res7,nil)

	// create a new folder and move the messages to it
	// done folder
	pm("create a new 'done' folder")

	u1_done_tmpuid := "u1_done"
	u1_done_id := "user1_"+randdata+"_done"
	u1_done_ty := TYPE_FOLDER
	u1_done_p := "privdata"
	u1_done_s1 := "test[a]{a}<v>"

	u1_done := cm.NewGraphNodeJustUID(u1_done_tmpuid)
	u1_done.Owner = u1user
	u1_done.Id = &u1_done_id
	u1_done.Type = &u1_done_ty
	u1_done.PrivateData = &u1_done_p
	u1_done.String1 = &u1_done_s1

	// edge from inbox -> done
	edge_u1inbox_done := cm.NewGraphNodeEdge(u1_inbox_uid, u1_done_tmpuid)
	
	upsertNodeList = &[]*cm.GraphNode{
		u1_done, 
		edge_u1inbox_done,
	}
	res8, err8 := db.UpsertNodes(upsertNodeList)
	pr(res8,err8)

	u1_done_uid := res8.CreatedNodes[u1_done_tmpuid].Uid
	u1_orders_msg1_uid := res6.CreatedNodes[u1_orders_msg1_tmpuid].Uid
	u1_orders_msg2_uid := res6.CreatedNodes[u1_orders_msg2_tmpuid].Uid

	// move messages from orders to done
	// func (db *DB) AddNodeEdges(nodeUids, incomingUids, outgoingUids *[]string) (*res.CoggedResponse, error) {
	pm("move messages from orders to done (add edges)")
	res9, err9 := db.AddNodeEdges(
		&[]string{u1_done_uid}, 
		&[]string{}, 
		&[]string{u1_orders_msg1_uid, u1_orders_msg2_uid},
	)
	pr(res9,err9)

	u1_orders_uid := res6.CreatedNodes[u1_orders_tmpuid].Uid
	
	// func (db *DB) RemoveNodeEdges(nodeUids, incomingUids, outgoingUids *[]string) (*res.CoggedResponse, error) {
	pm("move messages from orders to done (del edges)")
	res10, err10 := db.RemoveNodeEdges(
		&[]string{u1_orders_uid}, 
		&[]string{}, 
		&[]string{u1_orders_msg1_uid, u1_orders_msg2_uid},
	)
	pr(res10,err10)

	// run query again and verify nodes moved from orders to done
	pm("verify nodes moved from orders to done")
	res11 := db.QueryWithOptions(query7, svc.NODENODE)
	pr(res11,nil)


	// add user
	pm("add user2")
	u2_tmpuid := "u2"
	u2name := "user2_"+randdata
	u2pass := "u2pass"
	u2data := "u2data"
	u2intd := "u2intd"
	u2role := "u2role"
	u2 := &cm.GraphUser{
		GraphBase: cm.GraphBase{Uid: u2_tmpuid},
		Username: &u2name,
		PasswordHash: &u2pass,
		Data:  &u2data,
		InternalData: &u2intd,
		Role: &u2role,
	}
	upU2slice := make([]*cm.GraphUser,1)
	upU2slice[0] = u2
	res12, err12 := db.UpsertUsers(&upU2slice)
	pr(res12,err12)

	u2_uid := res12.CreatedUids[u2_tmpuid]

	// share u1_done with user2
	// func (db *DB) UpdateUserShareEdges(uidsOfNodesToShare, uidsOfUsersToShareWith *[]string, addOrDel UpdateType) (*res.CoggedResponse, error) {
	pm("share nodes ('done' folder) with user2")
	res13, err13 := db.UpdateUserShareEdges(
		&[]string{u1_done_uid},
		&[]string{u2_uid}, 
		svc.ADD,
	)
	pr(res13,err13)

	
	pm("query all user shared nodes for user2")
	query14 := &req.QueryRequest{
		RootIDs: []string{u2_uid}, 
		Depth: uint(1), 
		Select: []string{"e","id","ty","s1","c","m"},
	}
	res14 := db.QueryWithOptions(query14, svc.USERSHARE)
	pr(res14,nil)


	// unshare nodes and query again
	pm("unshare nodes with user2")
	res15, err15 := db.UpdateUserShareEdges(
		&[]string{u1_done_uid},
		&[]string{u2_uid}, 
		svc.DELETE,
	)
	pr(res15,err15)

	
	pm("query all user shared nodes for user2")
	query16 := &req.QueryRequest{
		RootIDs: []string{u2_uid}, 
		Depth: uint(1), 
		Select: []string{"e","id","ty","s1","c","m"},
	}
	res16 := db.QueryWithOptions(query16, svc.USERSHARE)
	pr(res16,nil)

}
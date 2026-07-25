//go:build integration

package services_test

// Ported from the former tests/db.go manual script. Exercises the DB layer end-to-end
// against a real, ephemeral Dgraph (started by dbtest via testcontainers): user upserts,
// user-node creation, subgraph upserts, moving nodes between folders by adding/removing
// edges, and sharing/unsharing nodes with another user.
//
// Run with: go test -tags=integration ./services/...

import (
	"fmt"
	"testing"

	cm "cogged/models"
	req "cogged/requests"
	sec "cogged/security"
	svc "cogged/services"
	dbtest "cogged/services/dbtest"
)

const (
	typeInbox   = "inbox"
	typeFolder  = "folder"
	typeMessage = "message"
	typeOutbox  = "outbox"
)

func strp(s string) *string { return &s }

// findByID reports whether any node in the slice has the given id (`id` predicate).
func findByID(nodes []*cm.GraphNode, id string) bool {
	for _, n := range nodes {
		if n != nil && n.Id != nil && *n.Id == id {
			return true
		}
	}
	return false
}

// messageCountUnder counts message-typed nodes reachable from a folder via node->node edges.
func messageCountUnder(t *testing.T, db *svc.DB, folderUID string) int {
	t.Helper()
	q := &req.QueryRequest{
		RootIDs: []string{folderUID},
		Depth:   uint(20),
		Select:  []string{"id", "ty"},
		Filters: &req.QueryRequestClause{Field: "ty", Op: "eq", Val: typeMessage},
	}
	resp := db.QueryWithOptions(q, svc.NODENODE)
	if resp.Error != "" {
		t.Fatalf("messageCountUnder query error: %s", resp.Error)
	}
	return len(resp.ResultNodes)
}

func TestDBLayerScenario(t *testing.T) {
	db, _ := dbtest.MustStart(t)

	rnd, _ := sec.GenerateRandomBytes(5)
	suffix := fmt.Sprintf("%x", rnd)

	// --- user1 ---
	u1name := "user1_" + suffix
	u1 := &cm.GraphUser{
		GraphBase:    cm.GraphBase{Uid: "u1"},
		Username:     strp(u1name),
		PasswordHash: strp("u1pass"),
		Data:         strp("u1data"),
		InternalData: strp("u1intd"),
		Role:         strp("u1role"),
	}
	u1list := []*cm.GraphUser{u1}
	res, err := db.UpsertUsers(&u1list)
	if err != nil {
		t.Fatalf("UpsertUsers(user1): %v", err)
	}
	u1uid := res.CreatedUids["u1"]
	if u1uid == "" {
		t.Fatalf("UpsertUsers did not return a uid for user1: %+v", res.CreatedUids)
	}

	// query user1 by username and confirm it matches the upsert
	ures, err := db.QueryUser(u1name)
	if err != nil {
		t.Fatalf("QueryUser(user1): %v", err)
	}
	if ures.Error != "" || ures.User == nil {
		t.Fatalf("QueryUser(user1) returned no user: %+v", ures)
	}
	if ures.User.Uid != u1uid {
		t.Errorf("QueryUser uid = %q, want %q (from upsert)", ures.User.Uid, u1uid)
	}
	u1user := &cm.GraphUser{GraphBase: cm.GraphBase{Uid: u1uid}}

	// --- user1 nodes: inbox + outbox ---
	inbox := &cm.GraphNode{
		GraphBase:   cm.GraphBase{Uid: "u1_inbox"},
		Id:          strp("user1_" + suffix + "_inbox"),
		Type:        strp(typeInbox),
		PrivateData: strp("privdata inbox"),
		String1:     strp("test[a]{a}<v>"),
	}
	inboxRes, err := db.UpsertUserNode(inbox, u1uid)
	if err != nil {
		t.Fatalf("UpsertUserNode(inbox): %v", err)
	}
	inboxUID := inboxRes.CreatedNodes["new"].Uid
	if inboxUID == "" {
		t.Fatal("UpsertUserNode(inbox) returned empty uid")
	}

	outbox := &cm.GraphNode{
		GraphBase: cm.GraphBase{Uid: "u1_outbox"},
		Id:        strp("user1_" + suffix + "_outbox"),
		Type:      strp(typeOutbox),
		String1:   strp("second user node"),
	}
	if _, err := db.UpsertUserNode(outbox, u1uid); err != nil {
		t.Fatalf("UpsertUserNode(outbox): %v", err)
	}

	// query all of user1's nodes -> inbox and outbox
	unRes := db.QueryWithOptions(&req.QueryRequest{
		RootIDs: []string{u1uid},
		Depth:   uint(1),
		Select:  []string{"id", "ty"},
	}, svc.USERNODE)
	if unRes.Error != "" {
		t.Fatalf("query user nodes: %s", unRes.Error)
	}
	if !findByID(unRes.ResultNodes, *inbox.Id) || !findByID(unRes.ResultNodes, *outbox.Id) {
		t.Errorf("expected inbox and outbox in user nodes, got %d nodes", len(unRes.ResultNodes))
	}

	// --- subgraph under inbox: orders folder + two messages ---
	msg1 := cm.NewGraphNodeJustUID("u1_orders_msg1")
	msg1.Owner, msg1.Id, msg1.Type = u1user, strp("user1_"+suffix+"_orders_msg1"), strp(typeMessage)
	msg2 := cm.NewGraphNodeJustUID("u1_orders_msg2")
	msg2.Owner, msg2.Id, msg2.Type = u1user, strp("user1_"+suffix+"_orders_msg2"), strp(typeMessage)

	orders := cm.NewGraphNodeJustUID("u1_orders")
	orders.Owner, orders.Id, orders.Type = u1user, strp("user1_"+suffix+"_orders"), strp(typeFolder)
	orders.OutEdges = &[]*cm.GraphNode{
		cm.NewGraphNodeJustUID("u1_orders_msg1"),
		cm.NewGraphNodeJustUID("u1_orders_msg2"),
	}
	edgeInboxOrders := cm.NewGraphNodeEdge(inboxUID, "u1_orders")

	subgraph := []*cm.GraphNode{orders, msg1, msg2, edgeInboxOrders}
	sgRes, err := db.UpsertNodes(&subgraph)
	if err != nil {
		t.Fatalf("UpsertNodes(subgraph): %v", err)
	}
	ordersUID := sgRes.CreatedNodes["u1_orders"].Uid
	msg1UID := sgRes.CreatedNodes["u1_orders_msg1"].Uid
	msg2UID := sgRes.CreatedNodes["u1_orders_msg2"].Uid
	if ordersUID == "" || msg1UID == "" || msg2UID == "" {
		t.Fatalf("subgraph upsert missing uids: %+v", sgRes.CreatedNodes)
	}

	// the two messages should now be reachable under orders
	if got := messageCountUnder(t, db, ordersUID); got != 2 {
		t.Errorf("orders should contain 2 messages, got %d", got)
	}

	// --- new 'done' folder under inbox ---
	done := cm.NewGraphNodeJustUID("u1_done")
	done.Owner, done.Id, done.Type = u1user, strp("user1_"+suffix+"_done"), strp(typeFolder)
	edgeInboxDone := cm.NewGraphNodeEdge(inboxUID, "u1_done")
	doneList := []*cm.GraphNode{done, edgeInboxDone}
	doneRes, err := db.UpsertNodes(&doneList)
	if err != nil {
		t.Fatalf("UpsertNodes(done): %v", err)
	}
	doneUID := doneRes.CreatedNodes["u1_done"].Uid
	if doneUID == "" {
		t.Fatal("done folder upsert returned empty uid")
	}

	// --- move messages from orders to done ---
	if _, err := db.AddNodeEdges(&[]string{doneUID}, &[]string{}, &[]string{msg1UID, msg2UID}); err != nil {
		t.Fatalf("AddNodeEdges(done <- messages): %v", err)
	}
	if _, err := db.RemoveNodeEdges(&[]string{ordersUID}, &[]string{}, &[]string{msg1UID, msg2UID}); err != nil {
		t.Fatalf("RemoveNodeEdges(orders -/-> messages): %v", err)
	}

	// verify the move
	if got := messageCountUnder(t, db, doneUID); got != 2 {
		t.Errorf("after move, done should contain 2 messages, got %d", got)
	}
	if got := messageCountUnder(t, db, ordersUID); got != 0 {
		t.Errorf("after move, orders should contain 0 messages, got %d", got)
	}

	// --- user2 + sharing ---
	u2name := "user2_" + suffix
	u2 := &cm.GraphUser{
		GraphBase:    cm.GraphBase{Uid: "u2"},
		Username:     strp(u2name),
		PasswordHash: strp("u2pass"),
		Role:         strp("u2role"),
	}
	u2list := []*cm.GraphUser{u2}
	u2res, err := db.UpsertUsers(&u2list)
	if err != nil {
		t.Fatalf("UpsertUsers(user2): %v", err)
	}
	u2uid := u2res.CreatedUids["u2"]
	if u2uid == "" {
		t.Fatalf("UpsertUsers did not return a uid for user2: %+v", u2res.CreatedUids)
	}

	// share the 'done' folder with user2
	if _, err := db.UpdateUserShareEdges(&[]string{doneUID}, &[]string{u2uid}, svc.ADD); err != nil {
		t.Fatalf("UpdateUserShareEdges(add): %v", err)
	}
	sharedQuery := &req.QueryRequest{
		RootIDs: []string{u2uid},
		Depth:   uint(1),
		Select:  []string{"id", "ty"},
	}
	shared := db.QueryWithOptions(sharedQuery, svc.USERSHARE)
	if shared.Error != "" {
		t.Fatalf("query shared nodes: %s", shared.Error)
	}
	if !findByID(shared.ResultNodes, *done.Id) {
		t.Errorf("expected 'done' folder to be shared with user2, got %d nodes", len(shared.ResultNodes))
	}

	// unshare and confirm it is gone
	if _, err := db.UpdateUserShareEdges(&[]string{doneUID}, &[]string{u2uid}, svc.DELETE); err != nil {
		t.Fatalf("UpdateUserShareEdges(delete): %v", err)
	}
	unshared := db.QueryWithOptions(sharedQuery, svc.USERSHARE)
	if unshared.Error != "" {
		t.Fatalf("query shared nodes after unshare: %s", unshared.Error)
	}
	if findByID(unshared.ResultNodes, *done.Id) {
		t.Errorf("'done' folder should no longer be shared with user2")
	}
}

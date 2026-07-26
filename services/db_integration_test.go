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

// adminUAD returns a sys-role caller, for which QueryWithOptions applies no read-authz
// filter (so these scenario tests see the raw DB results, as before). The read-authz
// filter itself is covered by TestDBReadAuthzFilter.
func adminUAD() *sec.UserAuthData { return &sec.UserAuthData{Role: sec.SYS_ROLE} }

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
	resp := db.QueryWithOptions(q, svc.NODENODE, adminUAD(), nil)
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
	}, svc.USERNODE, adminUAD(), nil)
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
	shared := db.QueryWithOptions(sharedQuery, svc.USERSHARE, adminUAD(), nil)
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
	unshared := db.QueryWithOptions(sharedQuery, svc.USERSHARE, adminUAD(), nil)
	if unshared.Error != "" {
		t.Fatalf("query shared nodes after unshare: %s", unshared.Error)
	}
	if findByID(unshared.ResultNodes, *done.Id) {
		t.Errorf("'done' folder should no longer be shared with user2")
	}
}

// TestDBPagination creates a folder with several message children and verifies both
// offset-based (first/offset) and cursor-based (first/after) pagination against a real
// Dgraph.
func TestDBPagination(t *testing.T) {
	db, _ := dbtest.MustStart(t)

	rnd, _ := sec.GenerateRandomBytes(5)
	suffix := fmt.Sprintf("%x", rnd)

	// owner user
	u := &cm.GraphUser{
		GraphBase:    cm.GraphBase{Uid: "u"},
		Username:     strp("pageuser_" + suffix),
		PasswordHash: strp("pw"),
		Role:         strp("user"),
	}
	ulist := []*cm.GraphUser{u}
	ures, err := db.UpsertUsers(&ulist)
	if err != nil {
		t.Fatalf("UpsertUsers: %v", err)
	}
	owner := &cm.GraphUser{GraphBase: cm.GraphBase{Uid: ures.CreatedUids["u"]}}

	// a folder with n message children
	const n = 5
	folder := cm.NewGraphNodeJustUID("folder")
	folder.Owner, folder.Id, folder.Type = owner, strp("folder_"+suffix), strp(typeFolder)
	nodeList := []*cm.GraphNode{folder}
	edges := &[]*cm.GraphNode{}
	for k := 0; k < n; k++ {
		key := fmt.Sprintf("msg%d", k)
		m := cm.NewGraphNodeJustUID(key)
		m.Owner, m.Id, m.Type = owner, strp(fmt.Sprintf("msg_%s_%d", suffix, k)), strp(typeMessage)
		nodeList = append(nodeList, m)
		*edges = append(*edges, cm.NewGraphNodeJustUID(key))
	}
	folder.OutEdges = edges
	res, err := db.UpsertNodes(&nodeList)
	if err != nil {
		t.Fatalf("UpsertNodes: %v", err)
	}
	folderUID := res.CreatedNodes["folder"].Uid

	// paginated query of the folder's message descendants
	page := func(first int, offset *int, after string) []*cm.GraphNode {
		q := &req.QueryRequest{
			RootIDs: []string{folderUID},
			Depth:   uint(20),
			Select:  []string{"id", "ty"},
			Filters: &req.QueryRequestClause{Field: "ty", Op: "eq", Val: typeMessage},
			First:   &first,
		}
		if offset != nil {
			q.Offset = offset
		}
		if after != "" {
			q.After = &after
		}
		r := db.QueryWithOptions(q, svc.NODENODE, adminUAD(), nil)
		if r.Error != "" {
			t.Fatalf("page query error: %s", r.Error)
		}
		return r.ResultNodes
	}

	if got := messageCountUnder(t, db, folderUID); got != n {
		t.Fatalf("expected %d messages under folder, got %d", n, got)
	}

	// offset-based: pages of 2 must cover all n distinctly, with no overlap
	seen := map[string]bool{}
	for off := 0; off < n; off += 2 {
		o := off
		p := page(2, &o, "")
		if off < n-1 && len(p) != 2 {
			t.Errorf("offset %d: expected 2 results, got %d", off, len(p))
		}
		for _, node := range p {
			if node.Uid == "" {
				t.Error("paginated node missing uid")
			}
			if seen[node.Uid] {
				t.Errorf("offset paging returned duplicate uid %s", node.Uid)
			}
			seen[node.Uid] = true
		}
	}
	if len(seen) != n {
		t.Errorf("offset paging covered %d distinct nodes, want %d", len(seen), n)
	}

	// cursor-based: page1, then everything after page1's last uid must be disjoint
	p1 := page(2, nil, "")
	if len(p1) != 2 {
		t.Fatalf("cursor page1 expected 2, got %d", len(p1))
	}
	p2 := page(2, nil, p1[len(p1)-1].Uid)
	if len(p2) != 2 {
		t.Fatalf("cursor page2 (after %s) expected 2, got %d", p1[len(p1)-1].Uid, len(p2))
	}
	in1 := map[string]bool{p1[0].Uid: true, p1[1].Uid: true}
	for _, node := range p2 {
		if in1[node.Uid] {
			t.Errorf("cursor page2 overlaps page1 at uid %s", node.Uid)
		}
	}
}

// TestDBReadAuthzFilter verifies that a NODENODE query pushes the read-permission check
// into DQL: a non-owner only sees nodes whose share-group they've been granted AND that
// carry the r permission — so paginated pages contain only readable nodes.
func TestDBReadAuthzFilter(t *testing.T) {
	db, _ := dbtest.MustStart(t)
	rnd, _ := sec.GenerateRandomBytes(5)
	suffix := fmt.Sprintf("%x", rnd)

	mkUser := func(key, name string) string {
		u := &cm.GraphUser{
			GraphBase:    cm.GraphBase{Uid: key},
			Username:     strp(name + "_" + suffix),
			PasswordHash: strp("pw"),
			Role:         strp("user"),
		}
		list := []*cm.GraphUser{u}
		res, err := db.UpsertUsers(&list)
		if err != nil {
			t.Fatalf("UpsertUsers(%s): %v", key, err)
		}
		return res.CreatedUids[key]
	}
	ownerUID := mkUser("owner", "owner")
	readerUID := mkUser("reader", "reader")
	owner := &cm.GraphUser{GraphBase: cm.GraphBase{Uid: ownerUID}}
	yes := true

	mk := func(key, id, sgi string, read bool) *cm.GraphNode {
		m := cm.NewGraphNodeJustUID(key)
		m.Owner, m.Id, m.Type = owner, strp(id+"_"+suffix), strp(typeMessage)
		g := sgi
		m.Sgi = &g
		if read {
			m.PermRead = &yes
		}
		return m
	}
	folder := cm.NewGraphNodeJustUID("folder")
	folder.Owner, folder.Id, folder.Type = owner, strp("folder_"+suffix), strp(typeFolder)
	folder.OutEdges = &[]*cm.GraphNode{
		cm.NewGraphNodeJustUID("m1"), cm.NewGraphNodeJustUID("m2"), cm.NewGraphNodeJustUID("m3"),
	}
	nodeList := []*cm.GraphNode{
		folder,
		mk("m1", "readable", "grantsgi", true), // granted sgi + read  -> visible to reader
		mk("m2", "noread", "grantsgi", false),  // granted sgi, no read -> hidden
		mk("m3", "othersgi", "othersgi", true), // ungranted sgi        -> hidden
	}
	res, err := db.UpsertNodes(&nodeList)
	if err != nil {
		t.Fatalf("UpsertNodes: %v", err)
	}
	folderUID := res.CreatedNodes["folder"].Uid

	reader := &sec.UserAuthData{Uid: readerUID, Role: "user"}
	readerQuery := func(sgis []string, first, offset *int) []*cm.GraphNode {
		q := &req.QueryRequest{
			RootIDs: []string{folderUID},
			Depth:   uint(20),
			Select:  []string{"id", "ty"},
			Filters: &req.QueryRequestClause{Field: "ty", Op: "eq", Val: typeMessage},
			First:   first,
			Offset:  offset,
		}
		r := db.QueryWithOptions(q, svc.NODENODE, reader, sgis)
		if r.Error != "" {
			t.Fatalf("reader query error: %s", r.Error)
		}
		return r.ResultNodes
	}

	// reader granted only "grantsgi" -> sees just m1; m2 (no read) and m3 (other sgi) are
	// filtered out by the query itself.
	got := readerQuery([]string{"grantsgi"}, nil, nil)
	if len(got) != 1 || got[0].Id == nil || *got[0].Id != "readable_"+suffix {
		t.Fatalf("reader should see only the readable node, got %d: %+v", len(got), got)
	}

	// reader with no grants -> sees nothing (owns none of them)
	if got := readerQuery(nil, nil, nil); len(got) != 0 {
		t.Errorf("reader without grants should see 0 nodes, got %d", len(got))
	}

	// owner sees all three (matched by the owner branch), proving the filter is per-caller
	owns := db.QueryWithOptions(&req.QueryRequest{
		RootIDs: []string{folderUID},
		Depth:   uint(20),
		Select:  []string{"id"},
		Filters: &req.QueryRequestClause{Field: "ty", Op: "eq", Val: typeMessage},
	}, svc.NODENODE, &sec.UserAuthData{Uid: ownerUID, Role: "user"}, nil)
	if len(owns.ResultNodes) != 3 {
		t.Errorf("owner should see all 3 messages, got %d", len(owns.ResultNodes))
	}

	// with the read filter in the query, pagination is now exact: first:1 returns the one
	// readable node, and paging past it is empty (no unreadable nodes consume page slots).
	one := 1
	off1 := 1
	if p := readerQuery([]string{"grantsgi"}, &one, nil); len(p) != 1 {
		t.Errorf("first:1 should return the 1 readable node, got %d", len(p))
	}
	if p := readerQuery([]string{"grantsgi"}, &one, &off1); len(p) != 0 {
		t.Errorf("offset past the only readable node should be empty, got %d", len(p))
	}
}

// TestDBVectorSimilarity verifies the float32vector predicate + HNSW index and the
// similar_to search against a real Dgraph. Uses hardcoded 3-dim vectors (no embedding
// dependency): A=[1,0,0], B=[0,1,0], C=[0.9,0.1,0]; a query of [1,0,0] with topK=2
// should return A (identical) and C (close), never the orthogonal B.
func TestDBVectorSimilarity(t *testing.T) {
	db, _ := dbtest.MustStart(t)
	rnd, _ := sec.GenerateRandomBytes(5)
	suffix := fmt.Sprintf("%x", rnd)

	ures, err := db.UpsertUsers(&[]*cm.GraphUser{{
		GraphBase: cm.GraphBase{Uid: "owner"}, Username: strp("vecowner_" + suffix),
		PasswordHash: strp("pw"), Role: strp("user"),
	}})
	if err != nil {
		t.Fatalf("UpsertUsers: %v", err)
	}
	owner := &cm.GraphUser{GraphBase: cm.GraphBase{Uid: ures.CreatedUids["owner"]}}
	yes := true

	mk := func(key, id, vec string) *cm.GraphNode {
		n := cm.NewGraphNodeJustUID(key)
		n.Owner, n.Id, n.Type, n.PermRead, n.Vec = owner, strp(id+"_"+suffix), strp(typeMessage), &yes, strp(vec)
		return n
	}
	nodes := []*cm.GraphNode{
		mk("a", "vecA", "[1.0, 0.0, 0.0]"),
		mk("b", "vecB", "[0.0, 1.0, 0.0]"),
		mk("c", "vecC", "[0.9, 0.1, 0.0]"),
	}
	if _, err := db.UpsertNodes(&nodes); err != nil {
		t.Fatalf("UpsertNodes: %v", err)
	}

	ownerUAD := &sec.UserAuthData{Uid: ures.CreatedUids["owner"], Role: "user"}
	r := db.QueryWithOptions(&req.QueryRequest{
		Similar: &req.QuerySimilarity{Vector: "[1.0, 0.0, 0.0]", TopK: 2},
		Select:  []string{"id"},
	}, svc.NODENODE, ownerUAD, nil)
	if r.Error != "" {
		t.Fatalf("similarity query error: %s", r.Error)
	}
	if len(r.ResultNodes) != 2 {
		t.Fatalf("topK=2 should return 2 nodes, got %d", len(r.ResultNodes))
	}
	if !findByID(r.ResultNodes, "vecA_"+suffix) || !findByID(r.ResultNodes, "vecC_"+suffix) {
		t.Errorf("expected vecA and vecC among the 2 nearest, got %+v", r.ResultNodes)
	}
	if findByID(r.ResultNodes, "vecB_"+suffix) {
		t.Errorf("orthogonal vecB should not be in the top-2")
	}
}

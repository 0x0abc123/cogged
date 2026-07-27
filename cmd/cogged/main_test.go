//go:build integration

package main

// End-to-end handler test. Runs an ephemeral Dgraph via testcontainers (needs Docker).
// Run with:  go test -tags=integration ./cmd/cogged/...

import (
	"bytes"
	cm "cogged/models"
	req "cogged/requests"
	res "cogged/responses"
	sec "cogged/security"
	svc "cogged/services"
	dbtest "cogged/services/dbtest"
	state "cogged/state"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func dump(i interface{}) string {
	pb, _ := json.Marshal(i)
	return string(pb)
}

func pr(t *testing.T, res interface{}, err error) {
	//t.Logf("RespJson:\n%s\nErr:\n%s\n",dump(res),err)
	fmt.Printf("RespJson:\n%s\nErr:\n%s\n", dump(res), err)
}

type Environment struct {
	Config    *svc.Config
	DB        *svc.DB
	SecretKey string
	Username  string
	Password  string
	Random    string
}

func setupTestEnvironment(t *testing.T) Environment {
	// Boot an ephemeral Dgraph via testcontainers; cleanup is registered on t.
	db, _ := dbtest.MustStart(t)
	cfg := db.Configuration

	randbytes, _ := sec.GenerateRandomBytes(32)
	sk := sec.B64Encode(randbytes)

	guid, _ := sec.GenerateGuid()
	uname := "testuser_" + guid[:8]
	passwd, err := addNewUser(uname+",sys", db)
	if err != nil {
		panic(err)
	}
	return Environment{
		Config:    cfg,
		DB:        db,
		SecretKey: sk,
		Username:  uname,
		Password:  passwd,
		Random:    guid[:8],
	}
}

func makeRequest(
	t *testing.T,
	dh *DefaultHandler,
	inputData interface{},
	httpMethod string,
	path string,
	token string,
	expectCode int,
) *httptest.ResponseRecorder {
	// Create a request with the JSON payload
	var ioReader io.Reader = nil
	if inputData != nil {
		jsonData, err := json.Marshal(inputData)
		if err != nil {
			t.Fatal(err)
		}
		ioReader = bytes.NewBuffer(jsonData)
	}

	req, err := http.NewRequest(httpMethod, path, ioReader)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", token)
	}

	rr := httptest.NewRecorder()

	dh.ServeHTTP(rr, req)

	if status := rr.Code; status != expectCode {
		t.Errorf("handler returned wrong status code: got %v want %v", status, expectCode)
	}
	return rr
}

func CreateNode(
	uid string,
	id string,
	ty string,
	s1 string,
	edges []string,
) *cm.GraphNode {
	n_id := id
	n_ty := ty
	n_s1 := s1
	bt := true
	node := &cm.GraphNode{
		GraphBase: cm.GraphBase{Uid: uid},
		Id:        &n_id,
		Type:      &n_ty,
		String1:   &n_s1,
		PermRead:  &bt,
	}

	if len(edges) > 0 {
		(*node).OutEdges = &[]*cm.GraphNode{}
		for _, e := range edges {
			*((*node).OutEdges) = append(*((*node).OutEdges), cm.NewGraphNodeJustUID(e))
		}
	}
	return node
}

func TestDefaultHandler(t *testing.T) {
	testenv := setupTestEnvironment(t)

	state.UsmInit()
	state.UsmRun()

	dh := CreateDefaultHandler(testenv.Config, testenv.DB, testenv.SecretKey)

	// unauthenticated request to /health/status should return 200
	{
		inputData := map[string]interface{}{"key": "value"}
		rr := makeRequest(t, dh, inputData, "POST", "/health/status", "", http.StatusOK)
		pr(t, dump(rr), nil)
	}

	// unauthenticated request to /auth/clientconfig should return 200
	{
		rr := makeRequest(t, dh, nil, "GET", "/auth/clientconfig", "", http.StatusOK)
		pr(t, dump(rr), nil)
	}

	// Test unauthenticated request should return 401
	{
		rr := makeRequest(t, dh, nil, "GET", "/auth/check", "", http.StatusUnauthorized)
		pr(t, dump(rr), nil)
	}

	// test auth/login returns token
	{
		inputData := req.LoginRequest{
			Username: testenv.Username,
			Password: testenv.Password,
		}
		rr := makeRequest(t, dh, inputData, "POST", "/auth/login", "", http.StatusOK)
		pr(t, dump(rr), nil)

		var result res.TokenResponse
		err := json.Unmarshal(rr.Body.Bytes(), &result)
		if err != nil {
			t.Errorf("error decoding JSON response: %v", err)
		}
		bearerTokenAdmin := "Bearer " + result.Token

		// do auth/check again with token, should return 200 OK
		{
			// Create a request with the JSON payload
			rr := makeRequest(t, dh, nil, "GET", "/auth/check", bearerTokenAdmin, http.StatusOK)
			pr(t, dump(rr), nil)
		}

		// /auth/refresh with token, should return 200 OK
		{
			// Create a request with the JSON payload
			rr := makeRequest(t, dh, nil, "GET", "/auth/refresh", bearerTokenAdmin, http.StatusOK)
			pr(t, dump(rr), nil)
			var result res.TokenResponse
			err := json.Unmarshal(rr.Body.Bytes(), &result)
			if err != nil {
				t.Errorf("error decoding JSON response: %v", err)
			}
			pr(t, result.Token, nil)
		}

		userRole := "user"
		user1uname := "alice_" + testenv.Random
		user1psswd := "user1pass"

		//create user 1
		{
			inputData := req.CreateUserRequest{
				Username: user1uname,
				Password: user1psswd,
				Role:     userRole,
			}
			rr := makeRequest(t, dh, inputData, "PUT", "/admin/user", bearerTokenAdmin, http.StatusOK)
			pr(t, dump(rr), nil)

			var result res.CoggedResponse
			err := json.Unmarshal(rr.Body.Bytes(), &result)
			if err != nil {
				t.Errorf("error decoding JSON response: %v", err)
			}
			pr(t, dump(result), nil)

		}

		bearerTokenUser1 := ""
		// login as user1
		{
			inputData := req.LoginRequest{
				Username: user1uname,
				Password: user1psswd,
			}
			rr := makeRequest(t, dh, inputData, "POST", "/auth/login", "", http.StatusOK)
			pr(t, dump(rr), nil)

			var result res.TokenResponse
			err := json.Unmarshal(rr.Body.Bytes(), &result)
			if err != nil {
				t.Errorf("error decoding JSON response: %v", err)
			}
			bearerTokenUser1 = "Bearer " + result.Token
			pr(t, dump(result), nil)
		}

		// user1 put user node
		//chatsUid := ""
		chatsAD := ""
		{
			inputData := req.UserNodeRequest{
				Node: CreateNode("$1", "u1/chats", "chats", "user1 chats", []string{}),
			}
			rr := makeRequest(t, dh, inputData, "PUT", "/user/node", bearerTokenUser1, http.StatusOK)
			pr(t, dump(rr), nil)

			var result res.CoggedResponse
			err := json.Unmarshal(rr.Body.Bytes(), &result)
			if err != nil {
				t.Errorf("error decoding JSON response: %v", err)
			}
			pr(t, dump(result), nil)
			//chatsUid = result.CreatedNodes["new"].Uid
			chatsAD = result.CreatedNodes["new"].AuthzData
		}

		chatWithBobAD := ""
		{
			// put subgraph of nodes under user1 chats
			chatWithBob := CreateNode("$cwb", "alice/cwb", "chat", "alice and bob", []string{"$abmsg1", "$abmsg2"})
			aliceToBobMsg1 := CreateNode("$abmsg1", "alice/bob/1", "msg", "hello bob", []string{})
			aliceToBobMsg2 := CreateNode("$abmsg2", "alice/bob/2", "msg", "what up?", []string{})
			nodeList := []*cm.GraphNode{chatWithBob, aliceToBobMsg1, aliceToBobMsg2}
			inputData := req.CreateNodesRequest{
				Nodes: &nodeList,
			}
			rr := makeRequest(t, dh, inputData, "PUT", "/graph/nodes/"+chatsAD, bearerTokenUser1, http.StatusOK)
			pr(t, dump(rr), nil)

			var result res.CoggedResponse
			err := json.Unmarshal(rr.Body.Bytes(), &result)
			if err != nil {
				t.Errorf("error decoding JSON response: %v", err)
			}
			pr(t, dump(result), nil)
			chatWithBobAD = result.CreatedNodes["$cwb"].AuthzData
		}

		// Malformed create-node payloads that used to panic the handler on a nil
		// dereference and drop the connection. Each must come back as a normal response
		// carrying an error. ServeHTTP is called directly here, so a panic would fail the
		// test outright rather than being swallowed by net/http's per-connection recovery.
		{
			cases := []struct {
				name string
				body string
				code int // 400 when Validate() rejects it, 200-with-error from the DB layer
			}{
				// all uids are $-placeholders so Validate passes; the DB layer refuses it
				// because no node in the request defines $ghost
				{"edge to undefined temp uid", `{"nodes":[{"uid":"$x","ty":"msg","e":[{"uid":"$ghost"}]}]}`, http.StatusOK},
				// JSON null decodes to a nil *GraphNode, now rejected during validation
				{"null node", `{"nodes":[null]}`, http.StatusBadRequest},
				{"null node among valid", `{"nodes":[{"uid":"$x","ty":"msg"},null]}`, http.StatusBadRequest},
				{"null out-edge", `{"nodes":[{"uid":"$x","ty":"msg","e":[null]}]}`, http.StatusBadRequest},
				// two nodes sharing a placeholder used to collapse into one, losing a
				// node silently; all uids are valid placeholders so Validate passes
				{"duplicate placeholder", `{"nodes":[{"uid":"$x","ty":"msg","s1":"one"},{"uid":"$x","ty":"msg","s1":"two"}]}`, http.StatusOK},
			}
			for _, tc := range cases {
				rr := makeRequest(t, dh, json.RawMessage(tc.body), "PUT", "/graph/nodes/"+chatsAD, bearerTokenUser1, tc.code)
				if tc.code != http.StatusOK {
					continue
				}
				var result res.CoggedResponse
				if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
					t.Errorf("%s: response was not valid JSON: %v", tc.name, err)
					continue
				}
				if result.Error == "" {
					t.Errorf("%s: expected an error in the response, got %s", tc.name, dump(result))
				}
				if len(result.CreatedNodes) != 0 {
					t.Errorf("%s: nothing should have been created, got %s", tc.name, dump(result.CreatedNodes))
				}
			}
		}

		user2uname := "bob_" + testenv.Random
		user2psswd := "user2pass"
		user2uid := ""
		//create user 2
		{
			inputData := req.CreateUserRequest{
				Username: user2uname,
				Password: user2psswd,
				Role:     userRole,
			}
			rr := makeRequest(t, dh, inputData, "PUT", "/admin/user", bearerTokenAdmin, http.StatusOK)
			pr(t, dump(rr), nil)

			var result res.CoggedResponse
			err := json.Unmarshal(rr.Body.Bytes(), &result)
			if err != nil {
				t.Errorf("error decoding JSON response: %v", err)
			}
			pr(t, dump(result), nil)
			user2uid = result.CreatedUids["newuser"]

		}

		bearerTokenUser2 := ""
		// login as user2
		{
			inputData := req.LoginRequest{
				Username: user2uname,
				Password: user2psswd,
			}
			rr := makeRequest(t, dh, inputData, "POST", "/auth/login", "", http.StatusOK)
			pr(t, dump(rr), nil)

			var result res.TokenResponse
			err := json.Unmarshal(rr.Body.Bytes(), &result)
			if err != nil {
				t.Errorf("error decoding JSON response: %v", err)
			}
			bearerTokenUser2 = "Bearer " + result.Token
			pr(t, dump(bearerTokenUser2), nil)
			pr(t, dump(result), nil)
		}

		user2AuthzData := ""
		// user1 get user2 details, should return 200 OK
		{
			// Create a request with the JSON payload
			rr := makeRequest(t, dh, nil, "GET", "/user/uid/"+user2uid, bearerTokenUser1, http.StatusOK)
			pr(t, dump(rr), nil)

			var result res.UserResponse
			err := json.Unmarshal(rr.Body.Bytes(), &result)
			if err != nil {
				t.Errorf("error decoding JSON response: %v", err)
			}
			pr(t, dump(result), nil)
			user2AuthzData = result.User.AuthzData
		}

		//user1 share chat with user 2
		{
			inputData := req.ShareNodesRequest{
				Users: &[]string{user2AuthzData},
				Nodes: &[]string{chatWithBobAD},
			}
			rr := makeRequest(t, dh, inputData, "PUT", "/user/share", bearerTokenUser1, http.StatusOK)
			pr(t, dump(rr), nil)

			var result res.CoggedResponse
			err := json.Unmarshal(rr.Body.Bytes(), &result)
			if err != nil {
				t.Errorf("error decoding JSON response: %v", err)
			}
			pr(t, dump(result), nil)

		}

		//check user1 shared chat with user 2
		{
			rr := makeRequest(t, dh, inputData, "GET", "/graph/sharedwith/"+chatWithBobAD, bearerTokenUser1, http.StatusOK)
			pr(t, dump(rr), nil)

			var result res.CoggedResponse
			err := json.Unmarshal(rr.Body.Bytes(), &result)
			if err != nil {
				t.Errorf("error decoding JSON response: %v", err)
			}
			pr(t, dump(result), nil)

		}

		chatWithBobADForBob := ""
		// user2 query shared nodes
		{
			inputData := req.QueryRequest{
				RootIDs: []string{},
				Select:  []string{"id", "ty", "s1"},
				Filters: &req.QueryRequestClause{
					Field: "ty",
					Op:    "eq",
					Val:   "chat",
				},
			}
			rr := makeRequest(t, dh, inputData, "POST", "/user/nodes/shared", bearerTokenUser2, http.StatusOK)
			pr(t, dump(rr), nil)

			var result res.CoggedResponse
			err := json.Unmarshal(rr.Body.Bytes(), &result)
			if err != nil {
				t.Errorf("error decoding JSON response: %v", err)
			}
			pr(t, dump(result), nil)
			chatWithBobADForBob = result.ResultNodes[0].AuthzData
		}

		{
			inputData := req.QueryRequest{
				RootIDs: []string{chatWithBobADForBob},
				Depth:   20,
				Select:  []string{"id", "ty", "s1"},
				Filters: &req.QueryRequestClause{
					Field: "ty",
					Op:    "eq",
					Val:   "msg",
				},
			}
			rr := makeRequest(t, dh, inputData, "POST", "/graph/nodes", bearerTokenUser2, http.StatusOK)
			pr(t, dump(rr), nil)

			var result res.CoggedResponse
			err := json.Unmarshal(rr.Body.Bytes(), &result)
			if err != nil {
				t.Errorf("error decoding JSON response: %v", err)
			}
			pr(t, dump(result), nil)
		}

	}

	sgitest := sec.GenerateSgi()
	fmt.Println("sgitest: " + sgitest)

}

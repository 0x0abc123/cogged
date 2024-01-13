package main

// docker pull dgraph/standalone:v22.0.2
// docker run -it --rm --net=dbridge dgraph/standalone:v22.0.2
//  export COGGED_TEST_DB_HOST=10.20.0.4 ; go test

import (
	"os"
	"io"
	"fmt"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	cm "cogged/models"
	sec "cogged/security"
	svc "cogged/services"
	req "cogged/requests"
	res "cogged/responses"
)


func dump(i interface{}) string {
	pb, _ := json.Marshal(i)
	return string(pb)
}

func pr(t *testing.T, res interface{}, err error) {
	//t.Logf("RespJson:\n%s\nErr:\n%s\n",dump(res),err)
	fmt.Printf("RespJson:\n%s\nErr:\n%s\n",dump(res),err)
}


type Environment struct {
	Config		*svc.Config
	DB			*svc.DB
	SecretKey	string
	Username	string
	Password	string
	Random		string
}


func setupTestEnvironment() Environment {
	dbHost := os.Getenv("COGGED_TEST_DB_HOST")
	if dbHost == "" { dbHost = "localhost" }
	dbPort := os.Getenv("COGGED_TEST_DB_PORT")
	if dbPort == "" { dbPort = "9080" }

	cfg := svc.Config{}
	cfg["db.host"] = dbHost
	cfg["db.port"] = dbPort

	db := svc.NewDB(&cfg)

	randbytes, _ := sec.GenerateRandomBytes(32)
	sk := sec.B64Encode(randbytes)

	guid, _ := sec.GenerateGuid()
	uname := "testuser_"+guid[:8]
	passwd, err := addNewUser(uname+",sys", db)
	if err != nil {
		panic(err)
	}
	return Environment{
		Config: &cfg,
		DB: db,
		SecretKey: sk,
		Username: uname,
		Password: passwd,
		Random: guid[:8],
	}
}


func makeRequest(
	t 			*testing.T, 
	dh 			*DefaultHandler, 
	inputData 	interface{}, 
	httpMethod	string, 
	path		string,
	token 		string, 
	expectCode 	int,
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
	uid		string,
	id		string,
	ty		string,
	s1		string,
	edges	[]string,
) *cm.GraphNode {
	n_id := id
	n_ty := ty
	n_s1 := s1
	bt := true
	node := &cm.GraphNode{
		GraphBase: cm.GraphBase{Uid: uid},
		Id: &n_id,
		Type: &n_ty,
		String1: &n_s1,
		PermRead: &bt,
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
	testenv := setupTestEnvironment()
	dh := CreateDefaultHandler(testenv.Config, testenv.DB, testenv.SecretKey)

	// unauthenticated request to /health/status should return 200
	{
		inputData := map[string]interface{}{"key": "value"}
		rr := makeRequest(t, dh, inputData, "POST", "/health/status", "", http.StatusOK)
		pr(t, dump(rr),nil)
	}

	// unauthenticated request to /auth/clientconfig should return 200
	{
		rr := makeRequest(t, dh, nil, "GET", "/auth/clientconfig", "", http.StatusOK)
		pr(t, dump(rr),nil)
	}

	// Test unauthenticated request should return 401 
	{
		rr := makeRequest(t, dh, nil, "GET", "/auth/check", "", http.StatusUnauthorized)
		pr(t, dump(rr),nil)
	}

	// test auth/login returns token
	{
		inputData := req.LoginRequest{
			Username: testenv.Username, 
			Password: testenv.Password,
		}
		rr := makeRequest(t, dh, inputData, "POST", "/auth/login", "", http.StatusOK)
		pr(t, dump(rr),nil)

		var result res.TokenResponse
		err := json.Unmarshal(rr.Body.Bytes(), &result)
		if err != nil {
			t.Errorf("error decoding JSON response: %v", err)
		}
		bearerTokenAdmin := "Bearer "+result.Token

		// do auth/check again with token, should return 200 OK
		{
			// Create a request with the JSON payload
			rr := makeRequest(t, dh, nil, "GET", "/auth/check", bearerTokenAdmin, http.StatusOK)
			pr(t, dump(rr),nil)
		}		

		// /auth/refresh with token, should return 200 OK
		{
			// Create a request with the JSON payload
			rr := makeRequest(t, dh, nil, "GET", "/auth/refresh", bearerTokenAdmin, http.StatusOK)
			pr(t, dump(rr),nil)
			var result res.TokenResponse
			err := json.Unmarshal(rr.Body.Bytes(), &result)
			if err != nil {
				t.Errorf("error decoding JSON response: %v", err)
			}
			pr(t, result.Token, nil)
		}		

		userRole := "user"
		user1uname := "alice_"+testenv.Random
		user1psswd := "user1pass"

		//create user 1
		{
			inputData := req.CreateUserRequest{
				Username: user1uname, 
				Password: user1psswd,
				Role: userRole,
			}
			rr := makeRequest(t, dh, inputData, "PUT", "/admin/user", bearerTokenAdmin, http.StatusOK)
			pr(t, dump(rr),nil)
	
			var result res.CoggedResponse
			err := json.Unmarshal(rr.Body.Bytes(), &result)
			if err != nil {
				t.Errorf("error decoding JSON response: %v", err)
			}
			pr(t, dump(result),nil)
			
		}

		bearerTokenUser1 := ""
		// login as user1
		{
			inputData := req.LoginRequest{
				Username: user1uname, 
				Password: user1psswd,
			}
			rr := makeRequest(t, dh, inputData, "POST", "/auth/login", "", http.StatusOK)
			pr(t, dump(rr),nil)
	
			var result res.TokenResponse
			err := json.Unmarshal(rr.Body.Bytes(), &result)
			if err != nil {
				t.Errorf("error decoding JSON response: %v", err)
			}
			bearerTokenUser1 = "Bearer "+result.Token
			pr(t, dump(result),nil)
		}

		// user1 put user node
		//chatsUid := ""
		chatsAD := ""
		{
			inputData := req.UserNodeRequest{
				Node: CreateNode("$1","u1/chats","chats","user1 chats",[]string{}),
			}
			rr := makeRequest(t, dh, inputData, "PUT", "/user/node", bearerTokenUser1, http.StatusOK)
			pr(t, dump(rr),nil)
	
			var result res.CoggedResponse
			err := json.Unmarshal(rr.Body.Bytes(), &result)
			if err != nil {
				t.Errorf("error decoding JSON response: %v", err)
			}
			pr(t, dump(result),nil)
			//chatsUid = result.CreatedNodes["new"].Uid
			chatsAD = result.CreatedNodes["new"].AuthzData
		}

		chatWithBobAD := ""
		{
			// put subgraph of nodes under user1 chats
			chatWithBob := CreateNode("$cwb","alice/cwb","chat","alice and bob",[]string{"$abmsg1","$abmsg2"})
			aliceToBobMsg1 := CreateNode("$abmsg1","alice/bob/1","msg","hello bob",[]string{})
			aliceToBobMsg2 := CreateNode("$abmsg2","alice/bob/2","msg","what up?",[]string{})
			nodeList := []*cm.GraphNode{chatWithBob, aliceToBobMsg1, aliceToBobMsg2}
			inputData := req.CreateNodesRequest{
				Nodes: &nodeList,
			}
			rr := makeRequest(t, dh, inputData, "PUT", "/graph/nodes/"+chatsAD, bearerTokenUser1, http.StatusOK)
			pr(t, dump(rr),nil)

			var result res.CoggedResponse
			err := json.Unmarshal(rr.Body.Bytes(), &result)
			if err != nil {
				t.Errorf("error decoding JSON response: %v", err)
			}
			pr(t, dump(result),nil)
			chatWithBobAD = result.CreatedNodes["$cwb"].AuthzData
		}

		user2uname := "bob_"+testenv.Random
		user2psswd := "user2pass"
		user2uid := ""
		//create user 2
		{
			inputData := req.CreateUserRequest{
				Username: user2uname, 
				Password: user2psswd,
				Role: userRole,
			}
			rr := makeRequest(t, dh, inputData, "PUT", "/admin/user", bearerTokenAdmin, http.StatusOK)
			pr(t, dump(rr),nil)
	
			var result res.CoggedResponse
			err := json.Unmarshal(rr.Body.Bytes(), &result)
			if err != nil {
				t.Errorf("error decoding JSON response: %v", err)
			}
			pr(t, dump(result),nil)
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
			pr(t, dump(rr),nil)
	
			var result res.TokenResponse
			err := json.Unmarshal(rr.Body.Bytes(), &result)
			if err != nil {
				t.Errorf("error decoding JSON response: %v", err)
			}
			bearerTokenUser2 = "Bearer "+result.Token
			pr(t, dump(bearerTokenUser2),nil)
			pr(t, dump(result),nil)
		}

		user2AuthzData := ""
		// user1 get user2 details, should return 200 OK
		{
			// Create a request with the JSON payload
			rr := makeRequest(t, dh, nil, "GET", "/user/uid/"+user2uid, bearerTokenUser1, http.StatusOK)
			pr(t, dump(rr),nil)

			var result res.UserResponse
			err := json.Unmarshal(rr.Body.Bytes(), &result)
			if err != nil {
				t.Errorf("error decoding JSON response: %v", err)
			}
			pr(t, dump(result),nil)
			user2AuthzData = result.User.AuthzData
		}		

		//user1 share chat with user 2
		{
			inputData := req.ShareNodesRequest{
				Users: &[]string{user2AuthzData}, 
				Nodes: &[]string{chatWithBobAD},
			}
			rr := makeRequest(t, dh, inputData, "PUT", "/user/share", bearerTokenUser1, http.StatusOK)
			pr(t, dump(rr),nil)
	
			var result res.CoggedResponse
			err := json.Unmarshal(rr.Body.Bytes(), &result)
			if err != nil {
				t.Errorf("error decoding JSON response: %v", err)
			}
			pr(t, dump(result),nil)

		}

		chatWithBobADForBob := ""
		// user2 query shared nodes
		{
			inputData := req.QueryRequest{
				RootIDs: []string{}, 
				Select: []string{"id","ty","s1"},
				Filters: &req.QueryRequestClause{
					Field: "ty",
					Op: "eq",
					Val: "chat",
				},

			}
			rr := makeRequest(t, dh, inputData, "POST", "/user/nodes/shared", bearerTokenUser2, http.StatusOK)
			pr(t, dump(rr),nil)
	
			var result res.CoggedResponse
			err := json.Unmarshal(rr.Body.Bytes(), &result)
			if err != nil {
				t.Errorf("error decoding JSON response: %v", err)
			}
			pr(t, dump(result),nil)
			chatWithBobADForBob = result.ResultNodes[0].AuthzData
		}

		{
			inputData := req.QueryRequest{
				RootIDs: []string{chatWithBobADForBob},
				Depth: 20,
				Select: []string{"id","ty","s1"},
				Filters: &req.QueryRequestClause{
					Field: "ty",
					Op: "eq",
					Val: "msg",
				},

			}
			rr := makeRequest(t, dh, inputData, "POST", "/graph/nodes", bearerTokenUser2, http.StatusOK)
			pr(t, dump(rr),nil)
	
			var result res.CoggedResponse
			err := json.Unmarshal(rr.Body.Bytes(), &result)
			if err != nil {
				t.Errorf("error decoding JSON response: %v", err)
			}
			pr(t, dump(result),nil)
		}

	}


}


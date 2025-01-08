package client

import (
	"io"
	"fmt"
	"time"
	"bytes"
	"strings"
	"encoding/json"
	"net/http"
	//cm "cogged/models"
	req "cogged/requests"
	res "cogged/responses"
)

type ApiClientError struct {
	Info string
	StatusCode int
}

func (e ApiClientError) Error() string {
	return e.Info
}


type CoggedApiClient struct {
	Url		string
	Username	string
	Password	string
	authToken	string
	lastRequest	int64
	tokenExpirySec	int
}


var (

	unauthenticatedRoutes = map[string]bool{
		"POST /auth/login": true,
		"GET /auth/clientconfig": true,
		"GET /health/status": true,
	}

)


func (c *CoggedApiClient) makeHttpRequest(httpMethod, controller, endpoint, param string, requestData interface{}) (string, error) {

	var ioReader io.Reader = nil
	if requestData != nil {
		jsonData, err := json.Marshal(requestData)
		if err != nil {
			return "", &ApiClientError{Info: "invalid json supplied for requestData", StatusCode: 0}
		}
		ioReader = bytes.NewBuffer(jsonData)
	}

	url := fmt.Sprintf("%s/%s/%s",c.Url,controller,endpoint)
	if len(param) > 0 {
		url += "/"+param
	}

	req, err := http.NewRequest(httpMethod, url, ioReader)
	if err != nil {
		return "", &ApiClientError{Info: err.Error(), StatusCode: 0}
	}
	req.Header.Set("Content-Type", "application/json")

	endpointKey := fmt.Sprintf("%s %s/%s", strings.ToUpper(httpMethod), controller, endpoint)
	_, isUnauthenticatedRoute := unauthenticatedRoutes[endpointKey]

	if !isUnauthenticatedRoute && c.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.authToken)
	}

	client := &http.Client{}
	resp, errRes := client.Do(req)
	if errRes != nil {
		return "", &ApiClientError{Info: errRes.Error(), StatusCode: 0}
	}

	defer resp.Body.Close()
	body, errBody := io.ReadAll(resp.Body)

	if errBody != nil {
		return "", &ApiClientError{Info: errBody.Error(), StatusCode: 0}
	}
	if !isUnauthenticatedRoute {
		c.lastRequest = time.Now().Unix()
	}
	return string(body), nil
}


func bindToResponse[T any](jsonString string, responseStruct *T) error {
	err := json.Unmarshal([]byte(jsonString), responseStruct)
	if err == nil {
		return nil
	}

	return &ApiClientError{Info: "json unmarshal failed", StatusCode: 0}
}


func (c *CoggedApiClient) Test() bool {
	fmt.Printf("CoggedApiClient test %s\n",c.Url)
	return true
}

func (c *CoggedApiClient) Login() bool {
	lr := &req.LoginRequest{Username: c.Username, Password: c.Password}
	tr, err := c.authLoginPost(lr)
	success := (err == nil)
	if success {
		c.authToken = tr.Token
		c.tokenExpirySec = tr.Expires
	}
	return success
}

func (c *CoggedApiClient) Logout() bool {
	_, err := c.authLogoutPost()
	success := (err == nil)
	if success {
		c.authToken = ""
		c.tokenExpirySec = 0
		c.lastRequest = 0
	}
	return success
}


func (c *CoggedApiClient) authLoginPost(lr *req.LoginRequest) (*res.TokenResponse, error)  {
	r := &res.TokenResponse{}
	var err error = nil;
	var respBody string;
	if respBody, err = c.makeHttpRequest("POST", "auth", "login", "", lr); err == nil {
		err = bindToResponse[res.TokenResponse](respBody, r)
	}
	return r, err

/*
	if !lr.Validate() {
		return nil, &ApiClientError{Info: "invalid request data", StatusCode: 0}
	}
	respBody, errReq := c.makeHttpRequest("POST", "auth", "login", "", lr)
	if errReq != nil {
		return nil, errReq
	}
	r := &res.TokenResponse{}
	if berr := bindToResponse[res.TokenResponse](respBody, r); berr != nil {
		return nil, berr
	}
	return r, nil
*/
}

func (c *CoggedApiClient) authLogoutPost() (bool, error)  {
	_, err := c.makeHttpRequest("POST", "auth", "logout", "", &struct{}{})
	return err == nil, err
}

func (c *CoggedApiClient) AuthRefreshGet() (*res.TokenResponse, error)  {
	r := &res.TokenResponse{}
	var err error = nil;
	var respBody string;
	if respBody, err = c.makeHttpRequest("GET", "auth", "refresh", "", nil); err == nil {
		err = bindToResponse[res.TokenResponse](respBody, r)
	}
	return r, err
}

func (c *CoggedApiClient) AuthCheckGet() (bool, error)  {
	_, err := c.makeHttpRequest("GET", "auth", "check", "", nil)
	return err == nil, err
}

func (c *CoggedApiClient) AuthClientconfigGet() (*map[string]string, error)  {
	r := &map[string]string{}
	var err error = nil;
	var respBody string;
	if respBody, err = c.makeHttpRequest("GET", "auth", "clientconfig", "", r); err == nil {
		err = bindToResponse[map[string]string](respBody, r)
	}
	return r, err
}


func (c *CoggedApiClient) AdminUserPut(cur *req.CreateUserRequest) (*res.CoggedResponse, error)  {
	r := &res.CoggedResponse{}
	var err error = nil;
	var respBody string;
	if respBody, err = c.makeHttpRequest("PUT", "admin", "user", "", cur); err == nil {
		err = bindToResponse[res.CoggedResponse](respBody, r)
	}
	return r, err
}


func (c *CoggedApiClient) AdminUsersPatch(ur *req.UsersRequest) (*res.CoggedResponse, error)  {
	r := &res.CoggedResponse{}
	var err error = nil;
	var respBody string;
	if respBody, err = c.makeHttpRequest("PATCH", "admin", "users", "", ur); err == nil {
		err = bindToResponse[res.CoggedResponse](respBody, r)
	}
	return r, err
}


func (c *CoggedApiClient) GraphNodesPost(qr *req.QueryRequest) (*res.CoggedResponse, error)  {
	r := &res.CoggedResponse{}
	var err error = nil;
	var respBody string;
	if respBody, err = c.makeHttpRequest("POST", "graph", "nodes", "", qr); err == nil {
		err = bindToResponse[res.CoggedResponse](respBody, r)
	}
	return r, err
}

func (c *CoggedApiClient) GraphSharedwithGet(ad string) (*res.CoggedResponse, error)  {
	r := &res.CoggedResponse{}
	var err error = nil;
	var respBody string;
	if respBody, err = c.makeHttpRequest("GET", "graph", "sharedwith", ad, nil); err == nil {
		err = bindToResponse[res.CoggedResponse](respBody, r)
	}
	return r, err
}


func (c *CoggedApiClient) GraphNodesPatch(unr *req.UpdateNodesRequest) (*res.CoggedResponse, error)  {
	r := &res.CoggedResponse{}
	var err error = nil;
	var respBody string;
	if respBody, err = c.makeHttpRequest("PATCH", "graph", "nodes", "", unr); err == nil {
		err = bindToResponse[res.CoggedResponse](respBody, r)
	}
	return r, err
}

func (c *CoggedApiClient) GraphNodesPut(ad string, cnr *req.CreateNodesRequest) (*res.CoggedResponse, error)  {
	r := &res.CoggedResponse{}
	var err error = nil;
	var respBody string;
	if respBody, err = c.makeHttpRequest("PUT", "graph", "nodes", ad, cnr); err == nil {
		err = bindToResponse[res.CoggedResponse](respBody, r)
	}
	return r, err
}

func (c *CoggedApiClient) GraphEdgesPut(er *req.EdgesRequest) (*res.CoggedResponse, error)  {
	r := &res.CoggedResponse{}
	var err error = nil;
	var respBody string;
	if respBody, err = c.makeHttpRequest("PUT", "graph", "edges", "", er); err == nil {
		err = bindToResponse[res.CoggedResponse](respBody, r)
	}
	return r, err
}

func (c *CoggedApiClient) GraphEdgesPatch(er *req.EdgesRequest) (*res.CoggedResponse, error)  {
	r := &res.CoggedResponse{}
	var err error = nil;
	var respBody string;
	if respBody, err = c.makeHttpRequest("PATCH", "graph", "edges", "", er); err == nil {
		err = bindToResponse[res.CoggedResponse](respBody, r)
	}
	return r, err
}


func (c *CoggedApiClient) HealthStatusGet() (*map[string]string, error)  {
	r := &map[string]string{}
	var err error = nil;
	var respBody string;
	if respBody, err = c.makeHttpRequest("GET", "health", "status", "", r); err == nil {
		err = bindToResponse[map[string]string](respBody, r)
	}
	return r, err
}


func (c *CoggedApiClient) UserNodePut(unr *req.UserNodeRequest) (*res.CoggedResponse, error)  {
	r := &res.CoggedResponse{}
	var err error = nil;
	var respBody string;
	if respBody, err = c.makeHttpRequest("PUT", "user", "node", "", unr); err == nil {
		err = bindToResponse[res.CoggedResponse](respBody, r)
	}
	return r, err
}


func (c *CoggedApiClient) UserNodesOwnPost(qr *req.QueryRequest) (*res.CoggedResponse, error)  {
	r := &res.CoggedResponse{}
	var err error = nil;
	var respBody string;
	if respBody, err = c.makeHttpRequest("POST", "user", "nodes", "own", qr); err == nil {
		err = bindToResponse[res.CoggedResponse](respBody, r)
	}
	return r, err
}

func (c *CoggedApiClient) UserNodesSharedPost(qr *req.QueryRequest) (*res.CoggedResponse, error)  {
	r := &res.CoggedResponse{}
	var err error = nil;
	var respBody string;
	if respBody, err = c.makeHttpRequest("POST", "user", "nodes", "shared", qr); err == nil {
		err = bindToResponse[res.CoggedResponse](respBody, r)
	}
	return r, err
}


func (c *CoggedApiClient) UserSharePut(snr *req.ShareNodesRequest) (*res.CoggedResponse, error)  {
	r := &res.CoggedResponse{}
	var err error = nil;
	var respBody string;
	if respBody, err = c.makeHttpRequest("PUT", "user", "share", "", snr); err == nil {
		err = bindToResponse[res.CoggedResponse](respBody, r)
	}
	return r, err
}


func (c *CoggedApiClient) UserSharePatch(snr *req.ShareNodesRequest) (*res.CoggedResponse, error)  {
	r := &res.CoggedResponse{}
	var err error = nil;
	var respBody string;
	if respBody, err = c.makeHttpRequest("PATCH", "user", "share", "", snr); err == nil {
		err = bindToResponse[res.CoggedResponse](respBody, r)
	}
	return r, err
}


func (c *CoggedApiClient) UserNameGet(un string) (*res.UserResponse, error)  {
	r := &res.UserResponse{}
	var err error = nil;
	var respBody string;
	if respBody, err = c.makeHttpRequest("GET", "user", "name", un, nil); err == nil {
		err = bindToResponse[res.UserResponse](respBody, r)
	}
	return r, err
}

func (c *CoggedApiClient) UserUidGet(uid string) (*res.UserResponse, error)  {
	r := &res.UserResponse{}
	var err error = nil;
	var respBody string;
	if respBody, err = c.makeHttpRequest("GET", "user", "uid", uid, nil); err == nil {
		err = bindToResponse[res.UserResponse](respBody, r)
	}
	return r, err
}

package main

import (
	"os"
	"fmt"
	"time"
	"strconv"
	"flag"
	"strings"
	"reflect"
	"errors"
    "net/http"
	"io/ioutil"
	"cogged/log"
	"cogged/api"
	cm "cogged/models"
	svc "cogged/services"
	sec "cogged/security"
	state "cogged/state"
)

type Set map[string]bool

type DefaultHandler struct {
	health	api.HealthAPI
	auth	api.AuthAPI
	admin	api.AdminAPI
	graph	api.GraphAPI
	user	api.UserAPI
	allowList	*Set
	adminList	*Set
}


func (h *DefaultHandler) ErrorResponse(code int, message string, w http.ResponseWriter, r *http.Request) {
	text := message
	if len(message) < 1 { text = http.StatusText(code) }
	http.Error(w, text, code)
}


func (h *DefaultHandler) OkResponse(jsonString string, w http.ResponseWriter) {
    w.Header().Set("Content-Type", "application/json; charset=utf-8")
    fmt.Fprintf(w, jsonString)
}


func (h *DefaultHandler) checkTimestamp(timestamp string) bool {
	tokenEpoch, convErr := strconv.ParseInt(timestamp, 10, 64)
	if convErr!= nil { tokenEpoch = 0 }
	nowEpoch := time.Now().Unix()
	return nowEpoch - tokenEpoch < h.auth.TokenExpiry
}

func (h *DefaultHandler) checkTokenId(userid, tokenId string) bool {
	return state.UsmCheckTokenId(userid, tokenId)
}

func (h *DefaultHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// validate content type is JSON
	ctype := r.Header["Content-Type"]
	if len(ctype) != 1 || !strings.HasPrefix(ctype[0], "application/json") {
		h.ErrorResponse(http.StatusUnsupportedMediaType, "only JSON accepted", w, r)
		return
	}

	path := strings.TrimSpace(r.URL.Path)
	routeParts := strings.Split(path,"/")
	numParts := len(routeParts)

	// validate route is of one of the following formats:
	// /routegroup/endpoint
	// /routegroup/endpoint/:param

	if numParts < 2 {
		h.ErrorResponse(http.StatusBadRequest, "bad route format", w, r)
	} else {
		routeGroup := routeParts[1]
		if routeGroup == "" {
			h.ErrorResponse(http.StatusBadRequest, "bad route format", w, r)
			return
		}

		// validate auth token
		var userAuthData *sec.UserAuthData = nil

		authHdr := r.Header["Authorization"]
		if len(authHdr) == 1 && strings.HasPrefix(authHdr[0], "Bearer ") {
			// create user auth context struct from token, it will be passed to handlers' HandleRequest()
			tokStr := strings.Split(authHdr[0]," ")[1]
			userAuthData = sec.UADFromToken(tokStr, h.auth.SecretKey)

			if userAuthData != nil {
				log.Debug("userAuthData: %v\n",*userAuthData)

				// check the token timestamp and whether it has expired
				if !h.checkTimestamp(userAuthData.Timestamp) {
					state.UsmDeleteTokenId(userAuthData.Uid, userAuthData.TokenId)
					h.ErrorResponse(http.StatusUnauthorized, "token expired", w, r)
					return	
				}
				if !h.checkTokenId(userAuthData.Uid, userAuthData.TokenId) {
					h.ErrorResponse(http.StatusUnauthorized, "invalid token ID", w, r)
					return
				}
			} else {
				log.Debug("malformed token or invalid MAC:",tokStr)
			}
		} 
		
		// no valid token was present in request headers
		if userAuthData == nil {
			// check whether the requested route is on the unauthenticated allowlist
			if !(*h.allowList)[path] {
				h.ErrorResponse(http.StatusUnauthorized, "missing or invalid auth token", w, r)
				return	
			}
		}
		log.Debug("userauthdata",userAuthData)

		if (*h.adminList)[routeGroup] && !userAuthData.IsAdmin() {
			h.ErrorResponse(http.StatusUnauthorized, "", w, r)
			return	
		}

		reqBodyString := ""
		if r.Body != nil {
			bodybytes, _ := ioutil.ReadAll(r.Body)
			reqBodyString = string(bodybytes)	
		}

		var handlerResponseStr string = ""
		var handlerErr error

		handlerKey := r.Method + " " + routeParts[2]

		handlerParam := ""
		if numParts > 3 {
			handlerParam = routeParts[3]
		}

		var handler api.Handler

		switch routeGroup {
			case "health":
				handler = &h.health
			case "auth":
				handler = &h.auth
			case "admin":
				handler = &h.admin
			case "graph":
				handler = &h.graph
			case "user":
				handler = &h.user
			default:
				h.ErrorResponse(http.StatusNotFound, "", w, r)
				return
		}

		handlerResponseStr, handlerErr = handler.HandleRequest(handlerKey, handlerParam, reqBodyString, userAuthData)

		if handlerErr!= nil {
			statusCode := http.StatusInternalServerError
			metaValue := reflect.ValueOf(handlerErr).Elem()
			if sv := metaValue.FieldByName("StatusCode"); sv != (reflect.Value{}) {
				statusCode = int(sv.Int())
				if statusCode != 0 {
					statusCode = statusCode
				}	
			}
			msg := handlerErr.Error()
			h.ErrorResponse(statusCode, msg, w, r)
			log.Debug("handler error", handlerErr)
		} else {
			h.OkResponse(handlerResponseStr, w)
		}
	}
}


func loadAuthzSecretKey() []byte {
	// try getting from envionment variable
	passphraseString := os.Getenv("COGGED_KEY")
	if passphraseString == "" {
		// try current working directory
		currentDir, _ := os.Getwd()
		secretKeyFilePath := currentDir + "/cogged.key"
		_, err := os.Stat(secretKeyFilePath)
		if err == nil {
			secretKey, err := os.ReadFile(secretKeyFilePath)
			if err == nil {
				return secretKey
			}
		}
		passphraseString,_ = sec.GenerateGuid()
	}
	// generate key from COGGED_KEY or the random passphrase
	secretKey := sec.Argon2IDKey(passphraseString, sec.SHA512Hash([]byte(passphraseString)))
	return secretKey
}


func addNewUser(flagValue string, db *svc.DB) (string, error) {
	p := strings.Split(flagValue, ",")
	if len(p) < 2 {
		return "", errors.New("value for -adduser is 'username,role'")
	}
	u := strings.TrimSpace(p[0])
	if len(u) < 1 || strings.HasPrefix(u,"~") {
		return "", errors.New("username must be at least 1 char and cannot start with a tilde")
	}
	role := p[1]
	b,_ := sec.GenerateRandomBytes(32)
	password := sec.B64Encode(b)[:24]

	// add user to database
	upass := sec.GeneratePasswordHash(password)
	u1 := &cm.GraphUser{
		GraphBase: cm.GraphBase{Uid: "new"},
		Username: &u,
		PasswordHash: &upass,
		Role: &role,
	}
	upU1slice := make([]*cm.GraphUser,1)
	upU1slice[0] = u1
	_, err1 := db.UpsertUsers(&upU1slice)
	if err1 != nil {
		return "", err1
	}
	
	fmt.Printf("Added new user: %s, role: %s, password: %s\n", u, role, password)
	return password, nil
}


func CreateDefaultHandler(conf *svc.Config, db *svc.DB, skB64 string) *DefaultHandler {
	unauthenticatedRoutes := make(Set)
	unauthenticatedRoutes["/auth/login"] = true
	unauthenticatedRoutes["/auth/clientconfig"] = true
	unauthenticatedRoutes["/health/status"] = true

	adminRoutes := make(Set)
	adminRoutes["admin"] = true

	return &DefaultHandler{
		health:	*api.NewHealthAPI(),
		auth:	*api.NewAuthAPI(conf, db, skB64),
		admin:	*api.NewAdminAPI(conf, db),
		graph:	*api.NewGraphAPI(conf, db),
		user:	*api.NewUserAPI(conf, db),
		allowList: &unauthenticatedRoutes,
		adminList: &adminRoutes,
	}
}


func main() {
	var flagListenPort int
	flag.IntVar(&flagListenPort, "p", 0, "TCP Port that Cogged listens on (overrides config file)")
	
	var flagListenIP string
	flag.StringVar(&flagListenIP, "ip", "", "Interface that Cogged binds to to listen for incoming connections (overrides config file)")

	var flagConfigFile string
	flag.StringVar(&flagConfigFile, "conf", "", "Full filesystem path to config file (JSON)")

	var flagAddUser string
	flag.StringVar(&flagAddUser, "adduser", "", "Add a new Cogged user (supply 'username,role' as value, will generate and print a random password)")

	var flagDgraphHost string
	flag.StringVar(&flagDgraphHost, "dh", "", "URL for Dgraph host eg. 10.1.2.3 (overrides config file)")

	var flagDgraphPort string
	flag.StringVar(&flagDgraphPort, "dp", "", "URL for Dgraph port eg. 9080 (overrides config file)")

	flag.Parse()

	conf := svc.LoadConfig(flagConfigFile)

	if len(flagDgraphHost) > 0 {
		(*conf)["db.host"] = flagDgraphHost
	}
	if len(flagDgraphPort) > 0 {
		(*conf)["db.port"] = flagDgraphPort
	}

	db := svc.NewDB(conf)

	if len(flagAddUser) > 0 {
		_, err := addNewUser(flagAddUser, db)
		if err != nil {
			fmt.Println("failed to add user",err)
		}
		return
	}

	log.Info("cogged started, using config:", conf)

	sk := loadAuthzSecretKey()
	skB64 := sec.B64Encode(sk)

	state.UsmInit()
	state.UsmRun()
	state.UsmTest1()

	dh := CreateDefaultHandler(conf, db, skB64)

    mux := http.NewServeMux()
    mux.Handle("/", dh)
	
	listenOn := ""
	if flagListenIP != "" {
		listenOn = flagListenIP
	} else {
		listenOn = conf.Get("listen.host")
	}

	lp := ":8090"
	if flagListenPort > 0 {
		lp = fmt.Sprintf(":%d",flagListenPort)
	} else if clp := conf.Get("listen.port"); clp != "" {
		lp = ":" + clp
	}
	listenOn += lp
	
	fmt.Printf("Cogged started and listening on %s\n",listenOn)
    http.ListenAndServe(listenOn, mux)
}

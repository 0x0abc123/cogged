package state

import (
	"fmt"
	"strings"
	"cogged/log"
)

/*
*/


type UsmOp int
const (
	USM_TOKEN_GET		UsmOp = iota
	USM_TOKEN_ADD
	USM_TOKEN_DEL
	USM_TOKEN_PURGE
	USM_SGI_CHECK
	USM_SGI_ALLOW
	USM_SGI_REVOKE
	USM_REQRATE_LOGINFAILINC
	USM_REQRATE_LOGINFAILCOUNT
	USM_REQRATE_LOGINFAILRESET
)


type Set map[string]bool
type MapStringSet map[string]Set
type MapStringInt map[string]int

type UsmRequest struct {
    Operation	UsmOp
    UID		string
    Value	string
    ReturnVal	chan string
}


var (
    TokenIds		MapStringSet
    SgiAllowlist	MapStringSet
    FailedLogins	MapStringInt
    MsgsToUsm		chan UsmRequest
)


func makeMsg(op UsmOp, uid, val string, retvalch chan string) UsmRequest {
    return UsmRequest {
	Operation: op,
	UID: uid,
	Value: val,
	ReturnVal: retvalch,
    }
}

func UsmInit() {
	TokenIds = make(MapStringSet)
	SgiAllowlist = make(MapStringSet)
	FailedLogins = make(MapStringInt)
}

func UsmRun() {
	log.Debug("UsmRun")
	MsgsToUsm = make(chan UsmRequest)

	go func(msgch chan UsmRequest) {
		for msg := range msgch {
			fmt.Println(msg)
			switch msg.Operation {
			case USM_TOKEN_GET:
				if msg.UID != "" {
					tokenset, exists := TokenIds[msg.UID]
					if exists {
						_, exists2 := tokenset[msg.Value]
						if exists2 {
							msg.ReturnVal <- "OK"
							continue
						}
					}
				}
				msg.ReturnVal <- ""
			case USM_TOKEN_ADD:
				if msg.UID != "" {
					tokenset, exists := TokenIds[msg.UID]
					if !exists {
						tokenset = make(Set)
						TokenIds[msg.UID] = tokenset
					}
					tokenset[msg.Value] = true
				}
			case USM_TOKEN_DEL:
				if msg.UID != "" {
					tokenset, exists := TokenIds[msg.UID]
					if exists {
						delete(tokenset, msg.Value)
					}
				}
				msg.ReturnVal <- "OK"
			case USM_SGI_CHECK:
				if msg.UID != "" {
					allowlist, exists := SgiAllowlist[msg.UID]
					if exists {
						allowed, exists2 := allowlist[msg.Value]
						if exists2 && allowed {
							msg.ReturnVal <- "OK"
							continue
						}
					}
				}
				msg.ReturnVal <- ""
			case USM_SGI_ALLOW:
				if msg.UID != "" {
					allowlist, exists := SgiAllowlist[msg.UID]
					if !exists {
						allowlist = make(Set)
						SgiAllowlist[msg.UID] = allowlist
					}
					tp := strings.Split(msg.Value,",")
					if len(tp) > 0 {
						for _,p := range tp {
							if p != "" {
								allowlist[p] = true
							}
						}
					}
				}
				msg.ReturnVal <- ""
			case USM_SGI_REVOKE:
				if msg.UID != "" {
					allowlist, exists := SgiAllowlist[msg.UID]
					if exists {
						tp := strings.Split(msg.Value,",")
						if len(tp) > 0 {
							for _,p := range tp {
								if p != "" {
									delete(allowlist, p)
								}
							}
						}
					}
				}
			default:
				fmt.Println("default")
			}
		}
		close(msgch)
	}(MsgsToUsm)

}

func UsmTest1(){
	rvc := make(chan string)
	m := makeMsg(USM_TOKEN_GET,"uid3","val3",rvc)
	MsgsToUsm <- m
	rv := <- rvc
	fmt.Println("ReturnVal: "+rv)
}

func UsmCheckTokenId(userid, tokenId string) bool {
	rvc := make(chan string)
	m := makeMsg(USM_TOKEN_GET, userid, tokenId, rvc)
	MsgsToUsm <- m
	rv := <- rvc
	return (rv == "OK")
}

func UsmAddTokenId(userid, tokenId string) {
	m := makeMsg(USM_TOKEN_ADD, userid, tokenId, nil)
	MsgsToUsm <- m
}

func UsmDeleteTokenId(userid, tokenId string) bool {
	rvc := make(chan string)
	m := makeMsg(USM_TOKEN_DEL, userid, tokenId, rvc)
	MsgsToUsm <- m
	rv := <- rvc
	return (rv == "OK")
}

func UsmUserCanAccessSgi(userUid, sgi string) bool {
	rvc := make(chan string)
	m := makeMsg(USM_SGI_CHECK, userUid, sgi, rvc)
	MsgsToUsm <- m
	rv := <- rvc
	return (rv == "OK")
}

func UsmUserAllowlistSgi(userUid, sgi string) {
	rvc := make(chan string)
	m := makeMsg(USM_SGI_ALLOW, userUid, sgi, rvc)
	MsgsToUsm <- m
	<- rvc
}

func UsmUserRevokeSgi(userUid, sgi string) {
	m := makeMsg(USM_SGI_REVOKE, userUid, sgi, nil)
	MsgsToUsm <- m
}

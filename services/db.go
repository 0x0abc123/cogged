package services

import (
	"fmt"
	"strings"
	"strconv"
	"regexp"
	"time"
	"context"
	"encoding/json"

	"cogged/log"	
	cm "cogged/models"
	sec "cogged/security"
	res "cogged/responses"
	req "cogged/requests"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"github.com/dgraph-io/dgo/v210"
	"github.com/dgraph-io/dgo/v210/protos/api"
)

// go get -u -v github.com/dgraph-io/dgo/v210
// go get -u -v google.golang.org/grpc
// docker pull dgraph/standalone:v22.0.2
// docker run -it --rm --net=dbridge dgraph/standalone:v22.0.2
// https://dgraph.io/docs/v22.0/dql/clients/go/


type EdgeType int
const (
	NODENODE	EdgeType = iota
	USERNODE
	USERSHARE
	USERSHAREDWITH
)

type UpdateType int
const (
	ADD 		UpdateType = iota
	DELETE
)


const (
	OP_TEXTSEARCH string = "has"
	OP_EQ string = "eq"
	OP_GT string = "gt"
	OP_LT string = "lt"
	OP_GTE string = "gte"
	OP_LTE string = "lte"

	MAX_QUERY_RECURSE_DEPTH uint = 20
)

var (

	allowedOps = map[string]bool{
		OP_TEXTSEARCH: true,
		OP_EQ: true,
		OP_GT: true,
		OP_LT: true,
		OP_GTE: true,
		OP_LTE: true,
	}

	allowedFields = map[string]bool{
		"e": true,
		"ty": true,
		"id": true,
		"p": true,
		"s1": true,
		"s2": true,
		"s3": true,
		"s4": true,
		"b": true,
		"n1": true,
		"n2": true,
		"c": true,
		"m": true,
		"t1": true,
		"t2": true,
		"g": true,
	}

	rgxAlphaNumSpace	*regexp.Regexp
	rgxUid				*regexp.Regexp

	initialisedGlobal bool = false
)

type DBError struct {
	Info string
}

func (e DBError) Error() string {
	return e.Info
}


type CancelFunc func()

type DB struct {
	Configuration 	*Config
	client 			*dgo.Dgraph
	cCancel			CancelFunc
}


func initGlobal() {
	reA, _ := regexp.Compile("[A-Za-z0-9 ,]")
	reU, _ := regexp.Compile("^0x[0-9a-f]+$")
	rgxAlphaNumSpace = reA
	rgxUid = reU
	initialisedGlobal = true
}


func NewDB(conf *Config) *DB {
	if !initialisedGlobal {
		initGlobal()
	}

	newDB := &DB{
		Configuration: conf,
	}

	// create Dgraph client
	dbURL := fmt.Sprintf("%s:%s", newDB.Configuration.Get("db.host"), newDB.Configuration.Get("db.port"))
	err := newDB.Connect(dbURL)
	if err != nil {
		panic("Could not connect to Dgraph at "+dbURL)
		return nil	
	}
	newDB.MaybeUpdateSchema()

	return newDB
}


func getDgraphClient(dgraphAddress string) (*dgo.Dgraph, CancelFunc) {
	conn, err := grpc.Dial(dgraphAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err == nil {
		dc := api.NewDgraphClient(conn)
		dg := dgo.NewDgraphClient(dc)

		return dg, func() {
			if err := conn.Close(); err != nil {
				log.Error("Closing connection", err)
			}
		}
	}
	log.Error("Failed trying to dial gRPC", nil)
	return nil, func() {}
}


func (d *DB) Connect(connStr string) error {
	log.Debug("DB Connect "+connStr, nil)
	dc, cancelFunc := getDgraphClient(connStr)
	if dc == nil {
		return DBError{Info: "connect to DB failed"}
	}
	d.client = dc
	d.cCancel = cancelFunc
	return nil
}


func (d *DB) MaybeUpdateSchema() {
	latestSchemaVersion := GetDgraphSchemaVersionString()
	log.Info("latestSchemaVersion",latestSchemaVersion)
	q := `schema {}`

	rj,err := d.Query(q, nil)
	if err != nil {
		log.Error("query current Dgraph schema",err)
		return
	}

	if !strings.Contains(*rj, latestSchemaVersion) {
		op := &api.Operation{}
		op.Schema = CreateLatestSchema()
		log.Info("Need to alter Dgraph schema to latest version: ",latestSchemaVersion)
		ctx := context.Background()
		err := d.client.Alter(ctx, op)
		if err != nil {
			log.Error("alter Dgraph schema",err)
		}	
	} else {
		log.Info("Dgraph schema is already latest version", nil)
	}	
}

func (d *DB) DropAll() error {
	op := api.Operation{DropAll: true}
	ctx := context.Background()
	if err := d.client.Alter(ctx, &op); err != nil {
		return err
	}
	return nil
}


func (d *DB) Query(query string, vars *map[string]string) (*string, error) {
	log.Debug("DB Query and Vars", query, vars)
	ctx := context.Background()

	var resp *api.Response
	var err error
	if vars != nil {
		resp, err = d.client.NewTxn().QueryWithVars(ctx, query, *vars)
		if err != nil {
			log.Error("query with vars",err)
			return nil, err
		}	
	} else {
		resp, err = d.client.NewTxn().Query(ctx, query)
		if err != nil {
			log.Error("query",err)
			return nil, err
		}	
	}

	rj := string(resp.Json)
	return &rj, nil
}


func (d *DB) Mutate(i interface{}, SetOrDelete UpdateType) (*api.Response, error) {
	j,_ := json.Marshal(i)
	log.Debug("DB Mutate:",i)

	ctx := context.Background()

	mu := &api.Mutation{
		CommitNow: true,
	}

	if SetOrDelete == DELETE {
		mu.DeleteJson = j
	} else {
		mu.SetJson = j
	}
	response, err := d.client.NewTxn().Mutate(ctx, mu)
	if err != nil {
		log.Error("mutate",err)
		return nil, err
	}

	return response, nil
}


func escapeAllNonAlphanumOrSpaceChars(strVal string) string {
	retString := ""
	re := rgxAlphaNumSpace
	for _,c := range strVal {
		sc := string(c)
		if re.MatchString(sc) {
			retString += sc
		} else {
			retString += "\\"+sc
		}
	}
	return retString
}


func createRegex(strVal string) string {
	return "/" + escapeAllNonAlphanumOrSpaceChars(strVal) + "/i"
}


func renderOp(op string) string { 
	opL := strings.ToLower(op)
	if !allowedOps[opL] {
		opL = OP_TEXTSEARCH
	}
	return opL
}


func renderField(field string) string { 
	fL := strings.ToLower(field)
	if allowedFields[fL] {
		return fL
	}
	return ""
}


func constructQueryStringAndAddVars(clause req.QueryRequestClause, queryvars *map[string]string) string {
	retval := ""
	subclauses := []req.QueryRequestClause{}
	if clause.And != nil {
		subclauses = clause.And
	} else if clause.Or != nil {
		subclauses = clause.Or
	}

	if len(subclauses) > 0 {
		opstr := ""
		if clause.And != nil {
			opstr = " and "
		} else {
			opstr = " or "
		}
		clStrings := []string{}
		for _,subclause := range subclauses {
			clStrings = append(clStrings, constructQueryStringAndAddVars(subclause, queryvars))
		}
		retval = "(" + strings.Join(clStrings, opstr) + ")"

	} else {
		op := renderOp(clause.Op)
		field := renderField(clause.Field)
		clVal := clause.Val
		if clVal == "" {
			clVal = "0"
		}
		if field == "m" && clVal == "0" {
			epoch := time.Unix(0, 0)
			clVal = fmt.Sprintf("%s",epoch)
		}

		tmpHash := sec.MD5SumHex([]byte(clVal))
		valStr := fmt.Sprintf("$vv%s",tmpHash[:20])
		
		if op == OP_TEXTSEARCH {
			clVal = strings.TrimSpace(clVal)
			if len(clVal) > 2 {
				op = "regexp"
				clVal = createRegex(clVal)
			}
		}

		(*queryvars)[valStr] = clVal

		retval += op +"("+field+","+valStr+")"
	}
	return retval
}


func SanitiseUID(uid string) string {
	tmpUid := uid
	if startsWith0x := strings.HasPrefix(uid, "0x"); startsWith0x {
		tmpUid = uid[2:]
	}
	uFromHexString, _ := strconv.ParseUint(tmpUid, 16, 64)
	return fmt.Sprintf("0x%x",uFromHexString)
}


func sanitiseListOfUids(untrustedUidsList []string) []string {
	sanitisedUidList := []string{}
	for  _,s := range untrustedUidsList	{

		sanitisedUid := SanitiseUID(s)

		if sanitisedUid == "0x0" {
			continue
		}

		sanitisedUidList = append(sanitisedUidList, sanitisedUid)
	}
	return sanitisedUidList
}


func getEdgePredicateName(edgeType EdgeType) string {
	switch edgeType {
		case NODENODE:
			return "e"
		case USERNODE:
			return "nodes"
		case USERSHARE:
			return "shr"
		case USERSHAREDWITH:
			return "~shr"
		default:
			return ""
	}
}


func renderQueryVarsString(vars *map[string]string) string {
	tmpV := []string{}
	for key,_ := range *vars {
		if strings.HasPrefix(key, "$vv") {
			tmpV = append(tmpV, key)
		}
	}
	retval := ""
	tmpVLen := len(tmpV)
	if tmpVLen > 0 {
		switch {
			case tmpVLen > 1:
				retval = strings.Join(tmpV, ": string, ")
			default:
				retval = tmpV[0]
		}
		retval += ": string"
	}
	return retval
}


func renderFields(fields []string) string {
	tmpV := []string{}

	for _,field := range fields {
		if allowedFields[field] {
			if field == "e" {
				tmpV = append(tmpV, field+" {uid own {uid} sgi r w o i d s}")
			} else {
				tmpV = append(tmpV, field)
			}
		}
	}
	retval := ""
	tmpVLen := len(tmpV)
	if tmpVLen > 0 {
		switch {
			case tmpVLen > 1:
				retval = strings.Join(tmpV, " ")
			default:
				retval = tmpV[0]
		}
	}
	return retval
}


func SliceFromResultJSON[T any](j *string) *[]*T {
	type APIResult struct {
		QR  []*T `json:"qr"`
	}

	var a APIResult
	if err := json.Unmarshal([]byte(*j), &a); err != nil {
		log.Error("unmarshal query result",err)
		return nil
	}
	return &a.QR
}


func (d *DB) QueryWithOptions(q *req.QueryRequest, et EdgeType) *res.CoggedResponse {
	query := ""
	vars := make(map[string]string)

	if q.RootQuery != nil {
		query += `
		query q(__QVARS__) {
			qr(func:  __ROOTQUERY__)
		`
		query = strings.Replace(query, "__ROOTQUERY__", constructQueryStringAndAddVars(*q.RootQuery, &vars), -1)

	} else {
		recurseDepth := q.Depth
		switch {
		case recurseDepth > MAX_QUERY_RECURSE_DEPTH:
			recurseDepth = MAX_QUERY_RECURSE_DEPTH
		case recurseDepth < 0:
			recurseDepth = 0
		}
		
		//dgraph expects a string of uids like this: "[0x1, 0x2, 0x3]"
		//JsonSerializer.Serialize(uidsOfParentNodes) will quote each so the string is "[\"0x1\", \"0x2\", \"0x3\"]" 
		//this causes a parse error in dgraphQL
		// it seems to do its own validation but to be safe...

		uidsOfParentNodes := q.RootIDs

		if len(uidsOfParentNodes) < 1 {
			return res.CoggedResponseFromError("Root nodes list cannot be null or empty")
		}

		var sanitisedParentNodeList = sanitiseListOfUids(uidsOfParentNodes)

		/*!!!!! recent versions of Dgraph don't want any square brackets around the UIDs when calling uid()
		so it should be like this: uid(0x11,0x12,0x13), but v21.03.2 requires square brackets*/

		serialisedParentNodeList := "["+strings.Join(sanitisedParentNodeList,",")+"]"

		vars["$ids"] = serialisedParentNodeList

		if recurseDepth > 0	{
			vars["$rdepth"] = fmt.Sprintf("%d",recurseDepth)
			query = `query q($ids: string, $rdepth: int, __QVARS__) {
				var(func: uid($ids)) @recurse(depth: $rdepth) 
				{
				  NID as uid
				  __EDGETYPE__
				}
			  
				qr(func: uid(NID))`
			query = strings.Replace(query,"__EDGETYPE__", getEdgePredicateName(et), -1)
		} else {
			query = `query q($ids: string, __QVARS__) {
				qr(func: uid($ids))`
		}
	}
	if q.Filters == nil {
		epoch := time.Unix(0, 0)
		q.Filters = &req.QueryRequestClause{Field: "m", Op: "gt", Val: fmt.Sprintf("%s",epoch)}
	}
	query += `  @filter(__FILTERS__)
			{
				uid own {uid} sgi r w o i d s __FIELDS__
			}
		}`

	fields := ""
	if len(q.Select) > 0 {
		fields = renderFields(q.Select)
	}
	query = strings.Replace(query,"__FIELDS__",fields, -1)
	query = strings.Replace(query,"__FILTERS__",constructQueryStringAndAddVars(*q.Filters, &vars), -1)
	query = strings.Replace(query,"__QVARS__",renderQueryVarsString(&vars), -1)
	
	sp, err := d.Query(query, &vars)
	if err != nil {
		return res.CoggedResponseFromError("DB query failed")
	}
	nodesReturned := SliceFromResultJSON[cm.GraphNode](sp)
	resp := res.CoggedResponseFromNodes(nodesReturned)
	return resp
}


func MakeTempKeyFromString(s string, tmpkeyToGuidMap *map[string]string) string {
    var safeID string
    if guid, ok := (*tmpkeyToGuidMap)[s]; ok {
        safeID = guid 
    } else {
        safeID,_ = sec.GenerateGuid()
        (*tmpkeyToGuidMap)[s] = safeID
    }

    return "_" + safeID
}


func StoreNodeOutgoingEdgeData(oed *cm.NodePtrDictionary, srcNodeUID string, destNodeUID string) {
    destNode := &cm.GraphNode{GraphBase: cm.GraphBase{Uid: destNodeUID}}
    
    if sn, ok := (*oed)[srcNodeUID]; ok {
        
        if sn.OutEdges == nil {
            sn.OutEdges = &[]*cm.GraphNode{} 
        }
		oe := append(*sn.OutEdges, destNode)
        sn.OutEdges = &oe
        
    } else {
        
		tnow := time.Now().UTC()
        srcNode := &cm.GraphNode{
            GraphBase: cm.GraphBase{Uid: srcNodeUID},
            TimeModified: &tnow,
        }
        
        ldst := []*cm.GraphNode{destNode}
        srcNode.OutEdges = &ldst
        
        (*oed)[srcNodeUID] = srcNode
    }
}


func getUidOrSafeTempUid(_uid string) string {
	//the node UID must be 0xNN or a valid tempkey
	//regex match 0xNN and if not, then prepend _:safeid
	//this will allow a collablio client to do a node tree upsert
	//some user supplied tmpkeys can cause parsing errors, so replace all with guids

    id := strings.TrimSpace(_uid)

    if !rgxUid.MatchString(strings.ToLower(id)) {
        id = fmt.Sprintf("_:%s",sec.MD5SumHex([]byte(_uid)))
    }
	return id
}


func makeSafeUid(gb cm.GraphBaser, count int, safeKeyToTempKeyMap *map[string]string) {
	nUid := gb.GetUid()

	if strings.TrimSpace(nUid) == "" {
		gb.SetUid(fmt.Sprintf("_anon%d",count))
	}
	
	uidOrSafeUid := getUidOrSafeTempUid(nUid)
	if strings.HasPrefix(uidOrSafeUid, "_:") {
		(*safeKeyToTempKeyMap)[uidOrSafeUid[2:]] = nUid
	}
	gb.SetUid(uidOrSafeUid)
}


func makeSafeUids(dgraphNodeList *[]*cm.GraphNode, safeKeyToTempKeyMap *map[string]string) {
    for count, n := range *dgraphNodeList {
		var gb cm.GraphBaser = n
		makeSafeUid(gb, count, safeKeyToTempKeyMap)
    }
}


func makeSafeUserUids(dgraphUserList *[]*cm.GraphUser, safeKeyToTempKeyMap *map[string]string) {
    for count, n := range *dgraphUserList {
		var gb cm.GraphBaser = n
		makeSafeUid(gb, count, safeKeyToTempKeyMap)
    }
}


func ValidateUid(uid string) bool {
	return rgxUid.MatchString(strings.ToLower(uid))
}


func (db *DB) UpsertNodes(nodeList *[]*cm.GraphNode) (*res.CoggedResponse, error) {
	newUidsToReturn := make(cm.NodePtrDictionary)
	safeKeyToOriginalMap := make(map[string]string)
	originalKeyToNodeMap := make(cm.NodePtrDictionary)
  
	for _, n := range *nodeList {
	  originalKeyToNodeMap[n.Uid] = n
	}
  
	makeSafeUids(nodeList, &safeKeyToOriginalMap)
  
	for _, n := range *nodeList {
  
	  // existing outgoing edges will be retained in the node during upsert
	  if nOE := (*n).OutEdges; nOE != nil && len(*(nOE)) > 0 {
		for _, edgePtr := range *(nOE) {
		  uidOrSafeUid := getUidOrSafeTempUid((*edgePtr).Uid)
		  if strings.HasPrefix(uidOrSafeUid, "_:") {
			tempKey := uidOrSafeUid[2:]
			if _, ok := safeKeyToOriginalMap[tempKey]; !ok {
			  safeKeyToOriginalMap[tempKey] = (*edgePtr).Uid  
			}
		  }
		  (*edgePtr).Uid = uidOrSafeUid
		  (*edgePtr).AuthzData = ""
		}
	  }
  
	  // update lastModTime
	  tnow := time.Now().UTC()
	  n.TimeModified = &tnow
	  if strings.HasPrefix(n.Uid, "_:") {
		n.TimeCreated = n.TimeModified
		n.DgraphType = []string{"N"} 
	  }
	  n.AuthzData = ""
	}
	
	mr, err := db.Mutate(nodeList, ADD)
	if mr == nil || err != nil {
		return res.CoggedResponseFromError("DB operation failed"), err
	} 

	for k, v := range mr.Uids {
	  originalTempKey := safeKeyToOriginalMap[k]
	  newUid := v
	  originalNode := originalKeyToNodeMap[originalTempKey]

	  newNode := cm.NewGraphNodeJustOwnerAndPerms(originalNode)
	  newNode.Uid = newUid
  
	  newUidsToReturn[originalTempKey] = newNode
	}

	return res.CoggedResponseFromNodesMap(&newUidsToReturn), nil
}
  

func (db *DB) AddIncomingEdges(nodeUids, incomingUids *[]string, mutateList *[]cm.GraphNode, updateLastModTime bool) error {
	var lastModTime *time.Time = nil
	if updateLastModTime {
		tnow := time.Now().UTC()
		lastModTime = &tnow
	}

	if incomingUids != nil && len(*incomingUids) > 0 {
		cList := make([]*cm.GraphNode,0)
		for _,nodeUid := range *nodeUids {
			cList = append(cList, &cm.GraphNode{GraphBase: cm.GraphBase{Uid: nodeUid}, TimeModified: lastModTime} )
		}

		for _,parentUid := range *incomingUids {
			tmpN := cm.GraphNode{GraphBase: cm.GraphBase{Uid: parentUid}, TimeModified: lastModTime}
			tmpN.OutEdges = &cList
			*mutateList = append(*mutateList, tmpN )
		}
	}
	return nil
}


func (db *DB) AddOutgoingEdges(nodeUids, outgoingUids *[]string, mutateList *[]cm.GraphNode, updateLastModTime bool) error {
	var lastModTime *time.Time = nil
	if updateLastModTime {
		tnow := time.Now().UTC()
		lastModTime = &tnow
	}

	if outgoingUids != nil && len(*outgoingUids) > 0 {
		cList := make([]*cm.GraphNode,0)
		for _,outUid := range *outgoingUids {
			cList = append(cList, &cm.GraphNode{GraphBase: cm.GraphBase{Uid: outUid}, TimeModified: lastModTime} )
		}

		for _,nodeUid := range *nodeUids {
			tmpN := cm.GraphNode{GraphBase: cm.GraphBase{Uid: nodeUid}, TimeModified: lastModTime}
			tmpN.OutEdges = &cList
			*mutateList = append(*mutateList, tmpN )
		}
	}
	return nil
}


//link node
/*

assuming these nodes already exist:

0x121 0x123
	\  /
	0x122
	/  \  
0x124 0x125

{ "set":[
  {"uid":"0x121","children":[{"uid":"0x122"}]},
  {"uid":"0x123","children":[{"uid":"0x122"}]},
  {"uid":"0x122","children":[{"uid":"0x124"}]},
  {"uid":"0x122","children":[{"uid":"0x125"}]}
  ] }

List<NodeWithUidAndChildren>
*/

//unlink node
/*

assuming these nodes already exist:

0x121 0x123
	\  /
	0x122
	/  \  
0x124 0x125

{ "delete":[
  {"uid":"0x121","children":[{"uid":"0x122"}]},
  {"uid":"0x123","children":[{"uid":"0x122"}]},
  {"uid":"0x122","children":[{"uid":"0x124"}]},
  {"uid":"0x122","children":[{"uid":"0x125"}]}
  ] }

List<NodeWithUidAndChildren>
deleteJson : json
*/


func (db *DB) UpdateEdges(utype UpdateType, nodeUids, srcUids, destUids *[]string) (*res.CoggedResponse, error) {
	updateList := make([]cm.GraphNode,0)

	db.AddIncomingEdges(nodeUids, srcUids, &updateList, true)
	db.AddOutgoingEdges(nodeUids, destUids, &updateList, true)

	_, err := db.Mutate(updateList, utype)
	if err != nil {
		return res.CoggedResponseFromError("DB operation failed"), err
	} 

	// delete doesn't update any fields, so have to do another mutate TX to update the lastModTime for each of the nodes
	if utype == DELETE {
		setList := make([]cm.GraphNode,0)
		for _,uid := range *nodeUids {
			tnow := time.Now().UTC()
			setList = append(setList, cm.GraphNode{GraphBase: cm.GraphBase{Uid: uid}, TimeModified: &tnow} )
		}
		if srcUids != nil && len(*srcUids) > 0 {
			for _,uid := range *srcUids {
				tnow := time.Now().UTC()
				setList = append(setList, cm.GraphNode{GraphBase: cm.GraphBase{Uid: uid}, TimeModified: &tnow} )
			}
		}
		if destUids != nil && len(*destUids) > 0 {
			for _,uid := range *destUids {
				tnow := time.Now().UTC()
				setList = append(setList, cm.GraphNode{GraphBase: cm.GraphBase{Uid: uid}, TimeModified: &tnow} )
			}
		}

		_, err2 := db.Mutate(setList, ADD)
		if err2 != nil {
			return res.CoggedResponseFromError("DB operation failed"), err2
		} 	
	}

	return res.CoggedResponseFromNodes(nil), nil
}


func (db *DB) AddNodeEdges(nodeUids, incomingUids, outgoingUids *[]string) (*res.CoggedResponse, error) {
	return db.UpdateEdges(
		ADD,
		nodeUids, 
		incomingUids, 
		outgoingUids,
	)
}


func (db *DB) RemoveNodeEdges(nodeUids, incomingUids, outgoingUids *[]string) (*res.CoggedResponse, error) {
	return db.UpdateEdges(
		DELETE,
		nodeUids, 
		incomingUids, 
		outgoingUids,
	)
}


func (db *DB) QueryUser(username string) (*res.UserResponse, error) {
	vars := map[string]string{
	  "$username": username,
	}
  
	query := `
	  query q($username: string){
		qr(func: eq(un, $username)) @filter(type(U)) {
		  uid 
		  un 
		  ph
		  us
		  role
		}
	  }
	`

	sp, err := db.Query(query, &vars)
	if err != nil {
		return res.UserResponseFromError("DB query failed"), err
	}
	usersReturned := SliceFromResultJSON[cm.GraphUser](sp)
	if len(*usersReturned) < 1 {
		return res.UserResponseFromError("no result"), err
	}
	user := (*usersReturned)[0]
	resp := res.UserResponseFromUser(user)
	return resp, nil
}


func (db *DB) QueryUserByUid(userUid string, internalQuery bool) (*res.UserResponse, error) {
	vars := map[string]string{
	  "$useruid": SanitiseUID(userUid), 
	}
  
	internalData := ""
	if internalQuery {
	  internalData = "intd"
	}
  
	query := `
	  query q($useruid: string){
		qr(func: uid($useruid)) @filter(type(U)) {
		  uid 
		  un
		  ph
		  us
		  role
		  ` + internalData + `
		}
	  }
	`
	sp, err := db.Query(query, &vars)
	if err != nil {
		return res.UserResponseFromError("DB query failed"), err
	}
	usersReturned := SliceFromResultJSON[cm.GraphUser](sp)
	if len(*usersReturned) < 1 {
		return res.UserResponseFromError("no result"), err
	}
	user := (*usersReturned)[0]
	resp := res.UserResponseFromUser(user)
	return resp, nil
}

func (db *DB) QueryUsersThatNodeIsSharedWith(nodeUid string) (*res.CoggedResponse, error) {
	vars := map[string]string{
	  "$nodeid": SanitiseUID(nodeUid),
	}

	query := `
	  query q($nodeid: string) {
		var(func: uid($nodeid)) @recurse(depth: 1) 	{
		NID as uid
		`+getEdgePredicateName(USERSHAREDWITH)+`
		}

		qr(func: uid(NID)) @filter(type(U)) {
		uid
		un
		role
		}
	}`

	sp, err := db.Query(query, &vars)
	if err != nil {
		return res.CoggedResponseFromError("DB query failed"), err
	}
	usersReturned := SliceFromResultJSON[cm.GraphUser](sp)
	if len(*usersReturned) < 1 {
		return res.CoggedResponseFromError("no result"), err
	}

	resp := res.CoggedResponseFromUsers(usersReturned)
	return resp, nil
}


func (db *DB) UpsertUsers(users *[]*cm.GraphUser) (*res.CoggedResponse, error) {
	newUidsToReturn := make(map[string]string)
	safeKeyToOriginalMap := make(map[string]string)
  
	makeSafeUserUids(users, &safeKeyToOriginalMap)
  
	for _, u := range *users {
	  if strings.HasPrefix(u.GetUid(), "_:") {
		u.DgraphType = []string{"U"}
	  }
	}
  
	mr, err := db.Mutate(*users, ADD)
	if mr == nil || err != nil {
		return res.CoggedResponseFromError("DB operation failed"), err
	} 

	for k, v := range mr.Uids {
		newUidsToReturn[safeKeyToOriginalMap[k]] = v
	}
  
	return res.CoggedResponseFromUidsMap(&newUidsToReturn), nil
}


func (db *DB) UpsertUserNode(node *cm.GraphNode, userUid string) (*res.CoggedResponse, error) {
    // Set node properties
    node.Uid = "_:new"
	tnow := time.Now().UTC()
    node.TimeModified = &tnow
    node.TimeCreated = node.TimeModified
    node.DgraphType = []string{"N"}
    node.Owner = &cm.GraphUser{GraphBase: cm.GraphBase{Uid: userUid}}
    node.OutEdges = nil
    
    // Create user with node
    user := cm.GraphUser{GraphBase: cm.GraphBase{Uid: userUid}, Nodes: &[]*cm.GraphNode{node}}

	mr, err := db.Mutate(user, ADD)
	if err != nil {
		return res.CoggedResponseFromError("DB operation failed"), err
	} 

    node.Uid = mr.Uids["new"]
    node.TimeModified = nil 
    node.TimeCreated = nil
    node.DgraphType = nil
    
    // Create response
    newUidAndNode := cm.NodePtrDictionary{
        "new": node,
    }

	return res.CoggedResponseFromNodesMap(&newUidAndNode), nil
}


func (db *DB) UpdateUserShareEdges(uidsOfNodesToShare, uidsOfUsersToShareWith *[]string, addOrDel UpdateType) (*res.CoggedResponse, error) {
	// Create list of shared nodes
	var sharedNodesList []*cm.GraphNode
	for _, uid := range *uidsOfNodesToShare {
	  sharedNodesList = append(sharedNodesList, &cm.GraphNode{GraphBase: cm.GraphBase{Uid: uid}}) 
	}
  
	// Create list of users
	var otherUsersList []*cm.GraphUser
	for _, uid := range *uidsOfUsersToShareWith {
	  otherUsersList = append(otherUsersList, &cm.GraphUser{GraphBase: cm.GraphBase{Uid: uid}, Shared: &sharedNodesList})
	}

	_, err := db.Mutate(otherUsersList, addOrDel)
	if err != nil {
		return res.CoggedResponseFromError("DB operation failed"), err
	} 	

	return res.CoggedResponseFromNodes(nil), nil
}
  

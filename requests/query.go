package requests

import (
	"cogged/log"
	cm "cogged/models"
	sec "cogged/security"
)

type QueryRequestClause struct {
	And   []QueryRequestClause `json:"and,omitempty"`
	Or    []QueryRequestClause `json:"or,omitempty"`
	Field string               `json:"field,omitempty"`
	Op    string               `json:"op,omitempty"`
	Val   string               `json:"val,omitempty"`

	// Geo, when set, makes this clause a radius test on the `g` predicate instead of a
	// Field/Op/Val comparison — so proximity can be ANDed and ORed with ordinary filters
	// and applied to a root_ids traversal, which QueryRequest.Geo (a root function)
	// cannot. Field/Op/Val must be left unset on a geo clause.
	Geo *QueryGeo `json:"geo,omitempty"`
}

type QueryRequest struct {
	RootIDs   []string            `json:"root_ids,omitempty"`
	RootQuery *QueryRequestClause `json:"root_query,omitempty"`
	Depth     uint                `json:"depth"`
	Filters   *QueryRequestClause `json:"filters,omitempty"`
	Select    []string            `json:"select,omitempty"`

	// Pagination (all optional). First/Offset give offset-based paging; First/After
	// give cursor-based paging (After is a node uid, results after it in the ordering).
	// OrderBy names an indexed predicate to sort by (OrderDesc for descending); when
	// unset, results follow Dgraph's default uid order.
	//
	// The read-permission check is applied inside the query (see renderReadAuthzFilter in
	// services/db.go), so paginated results contain only nodes the caller may read.
	First     *int    `json:"first,omitempty"`
	Offset    *int    `json:"offset,omitempty"`
	After     *string `json:"after,omitempty"`
	OrderBy   *string `json:"order_by,omitempty"`
	OrderDesc bool    `json:"order_desc,omitempty"`

	// Similar, when set, runs a vector-similarity search over the `vec` predicate
	// (HNSW index) instead of a uid/root traversal: it returns the nodes nearest to
	// the given query vector. Results are still scoped by the caller's read
	// permissions and any Filters/Select. See QuerySimilarity.
	Similar *QuerySimilarity `json:"similar,omitempty"`

	// Geo, when set, runs a radius search over the `g` predicate (geo index) instead of
	// a uid/root traversal. Like Similar it replaces the root function, so RootIDs and
	// Depth are ignored; Filters and Select still apply, and results are still scoped by
	// the caller's read permissions. Geo and Similar cannot be combined. See QueryGeo.
	Geo *QueryGeo `json:"geo,omitempty"`
}

// QueryGeo requests a radius search: every node whose `g` point lies within Distance
// metres of Point.
//
// This is a containment test, not a nearest-neighbour search. Dgraph returns geo matches
// in uid order and cannot sort by distance (`orderasc: g` is rejected outright with
// "Value of type: geo isn't sortable"), nor does it return the computed distance. So
// pairing this with First does NOT give "the N nearest" — it gives an arbitrary N of the
// nodes inside the radius. If a caller needs nearest-first, they must fetch the whole
// radius and sort client-side.
type QueryGeo struct {
	// Point is the centre of the search as [longitude, latitude] — longitude FIRST,
	// matching GeoJSON and the format stored in a node's `g` predicate.
	Point []float64 `json:"point"`
	// Distance is the search radius in metres. Must be > 0.
	Distance float64 `json:"distance"`
}

// QuerySimilarity requests a vector-similarity search.
type QuerySimilarity struct {
	// Vector is the query embedding as a string-encoded float array, e.g.
	// "[0.1,0.2,0.3]" (the same format stored in a node's `vec` predicate).
	Vector string `json:"vector"`
	// TopK is how many nearest neighbours to retrieve (before read filtering).
	TopK uint `json:"top_k,omitempty"`
}

func (req *QueryRequest) AuthzDataUnpack(uad sec.UserAuthData, permissionsRequired string) bool {
	log.Debug("QueryRes AuthzDataUnpack", req)
	// only system role can supply a root query:
	if uad.Role != sec.SYS_ROLE && req.RootQuery != nil {
		return false
	}
	// allowe empty root IDs (for system users running a rootQuery or POST /user/nodes/shared|own)
	if len(req.RootIDs) < 1 {
		return true
	}
	return cm.AuthzDataUnpackADStringSlice(&req.RootIDs, uad, permissionsRequired)
}

func (req *QueryRequest) Validate() bool {
	return true
}

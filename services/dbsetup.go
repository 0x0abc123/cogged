package services

import (
    "fmt"
    sec "cogged/security"
)

const DGRAPH_SCHEMA = `
un: string @index(trigram, term) .
ph: string .
us: string @index(trigram, term) .
intd: string @index(trigram, term) .
role: string @index(trigram, term) .
nodes: [uid] .
shr: [uid] @reverse .
own: uid .
r: bool .
w: bool .
o: bool .
i: bool .
d: bool .
s: bool .
e: [uid] @reverse .
ty: string @index(hash) .
id: string @index(trigram, term) .
p: string @index(hash) .
s1: string @index(trigram, term) .
s2: string @index(term) .
s3: string @index(hash) .
s4: string @index(hash) .
b:  string .
n1: float .
n2: float .
c: datetime @index(hour) .
m: datetime @index(hour) .
t1: datetime @index(hour) .
t2: datetime @index(hour) .
g: geo .

type U {
    un
    ph
    us
    intd
    role
    nodes
    shr
}

type N {
    own
    r
    w
    o
    i
    d
    s
    e
    ty
    id
    p
    s1
    s2
    s3
    s4
    b
    n1
    n2
    c
    m
    t1
    t2
    g
}
`
func GetDgraphSchemaVersionString() string {
    return "cdsv_"+sec.MD5SumHex([]byte(DGRAPH_SCHEMA))[:8]
}

func CreateLatestSchema() string {
    return fmt.Sprintf("%s: string .\n%s", GetDgraphSchemaVersionString(), DGRAPH_SCHEMA)
}
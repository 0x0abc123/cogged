package responses

type AuthzDataPacker interface {
	AuthzDataPack(key string) 
}
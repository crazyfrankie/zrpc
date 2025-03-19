package server

type service struct {
	serviceImpl any
	method      map[string]*MethodDesc
}

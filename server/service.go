package server

import (
	"context"
	"go/ast"
	"log"
	"reflect"
)

type methodType struct {
	method    reflect.Method
	ArgType   reflect.Type
	ReplyType reflect.Type
}

func (m *methodType) newArgv() reflect.Value {
	// arg mus be a pointer type
	if m.ArgType.Kind() != reflect.Ptr {
		panic("unhandled default request")
	}

	return reflect.New(m.ArgType.Elem())
}

func (m *methodType) newReplyVal() reflect.Value {
	// reply must be a pointer type
	if m.ReplyType.Kind() != reflect.Ptr {
		panic("unhandled default response")
	}

	return reflect.New(m.ReplyType.Elem())
}

type service struct {
	name     string
	typ      reflect.Type
	receiver reflect.Value
	method   map[string]*methodType
}

func newService(receiver any) *service {
	s := new(service)
	s.receiver = reflect.ValueOf(receiver)
	s.name = reflect.Indirect(s.receiver).Type().Name()
	s.typ = reflect.TypeOf(receiver)
	if !ast.IsExported(s.name) {
		log.Fatalf("rpc server: %s is not a valid service name", s.name)
	}
	s.registerMethods()

	return s
}

func (s *service) call(m *methodType, argv reflect.Value) (reflect.Value, error) {
	f := m.method.Func
	ctx := reflect.ValueOf(context.Background())
	returnValues := f.Call([]reflect.Value{s.receiver, ctx, argv})
	if errInter := returnValues[1].Interface(); errInter != nil {
		return reflect.Value{}, errInter.(error)
	}
	return returnValues[0], nil
}

func (s *service) registerMethods() {
	s.method = make(map[string]*methodType)
	for i := 0; i < s.typ.NumMethod(); i++ {
		method := s.typ.Method(i)
		mType := method.Type
		if mType.NumIn() != 3 || mType.NumOut() != 2 {
			continue
		}
		if mType.Out(1) != reflect.TypeOf((*error)(nil)).Elem() {
			continue
		}
		argType, replyType := mType.In(2), mType.Out(0)
		if !isExportedOrBuiltinType(argType) || !isExportedOrBuiltinType(replyType) {
			continue
		}
		s.method[method.Name] = &methodType{
			method:    method,
			ArgType:   argType,
			ReplyType: replyType,
		}
		log.Printf("rpc server: register %s.%s\n", s.name, method.Name)
	}
}

func isExportedOrBuiltinType(t reflect.Type) bool {
	return ast.IsExported(t.Name()) || t.PkgPath() == ""
}

package ssconv

import (
	"errors"
	"fmt"
	"reflect"
)

var (
	ErrDstTypeNotReference   = errors.New("dst is supposed to be pointer,map or slice")
	ErrDstStructSrcNotStruct = errors.New("dst is struct while src is not")
	ErrDuplicateField        = errors.New("duplicate field")
)

const (
	ErrDstNotAddressable = "dst value is not addressable"
)

type ErrUnexpectedType struct {
	tp reflect.Type
}

func (e *ErrUnexpectedType) Error() string {
	return fmt.Sprintf("unexpected type %s is not supported", e.tp.Name())
}

type ErrNilSrcPtr struct {
	src reflect.Value
}

func (e *ErrNilSrcPtr) Error() string {
	return fmt.Sprintf("value of %s in src is nil", e.src.Type())
}

type ErrUnableAssignType struct {
	srcType reflect.Type
	dstType reflect.Type
}

func typeName(tp reflect.Type) string {
	res := ""
	for tp.Kind() == reflect.Ptr {
		tp = tp.Elem()
		res += "*"
	}
	res += tp.Name()
	return res
}

func (e *ErrUnableAssignType) Error() string {
	return fmt.Sprintf("cant not assign %s in src to %s in dst", typeName(e.srcType), typeName(e.dstType))
}

package ssconv

import (
	"errors"
	"reflect"
)

type LocalRule struct {
}

type Options struct {
	DeepCopy   bool
	localRules []LocalRule
	hashCode   string
}

func (op *Options) SetDeepCode(deepCopy bool) *Options {
	op.DeepCopy = deepCopy
	return op
}

type ParamList struct {
}

type convState struct {
}

type convFunc func(c *convState, src reflect.Value, dst reflect.Value, options Options, list ParamList)

func Conv(src interface{}, dst interface{}, options Options, list ParamList) (err error) {
	defer func() {
		if r := recover(); r != nil {
			if convErr, ok := r.(ConvErr); ok {
				err = convErr.Err
			} else {
				panic(convErr)
			}
		}
	}()

	srcValue := reflect.Indirect(reflect.ValueOf(src))
	dstValue := reflect.Indirect(reflect.ValueOf(dst))

	if !dstValue.CanAddr() {
		convPanic(errors.New(ErrDstNotAddressable))
	}

	srcType := srcValue.Type()
	dstType := dstValue.Type()
	//fmt.Fprintln(os.Stderr, "1",dstType,srcType)
	//fmt.Fprint()

	c := new(convState)
	newConv(srcType, dstType, options, list)(c, srcValue, dstValue, options, list)
	return nil
}

func newConv(srcType, dstType reflect.Type, options Options, list ParamList) convFunc {

	/*if dstType.Kind() != reflect.Ptr && dstType.Kind() != reflect.Map && dstType.Kind() != reflect.Slice {
		panic(ErrDstTypeNotReference)
	}*/

	//fmt.Fprintln(os.Stderr, dstType,srcType)

	//options that working in this level should be divided into a new Options
	switch dstType.Kind() {
	case reflect.Bool:
		fallthrough
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		fallthrough
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		fallthrough
	case reflect.Float32, reflect.Float64:
		fallthrough
	case reflect.Complex64, reflect.Complex128:
		fallthrough
	case reflect.String:
		fallthrough
	case reflect.Array:
		return basicConverter

	case reflect.Ptr:
		return newPtrConverter(srcType, dstType, options, list)
	case reflect.Map:
		fallthrough
	case reflect.Slice:
		fallthrough
	case reflect.Interface:
		fallthrough
	case reflect.Struct:
		fallthrough
	case reflect.Func:
		fallthrough

	case reflect.Chan:
		return UnexpectedTypeConverter
		//not supported yet
	default:
		//not supported type
		return UnexpectedTypeConverter
	}
}

type ConvErr struct {
	Err error
}

func convPanic(err error) {
	panic(ConvErr{Err: err})
}

func UnexpectedTypeConverter(c *convState, src reflect.Value, dst reflect.Value, options Options, list ParamList) {
	convPanic(&ErrUnexpectedType{dst.Type()})
}

func basicConverter(c *convState, src reflect.Value, dst reflect.Value, options Options, list ParamList) {
	//

	if src.Kind() == reflect.Ptr && src.IsNil() {
		convPanic(&ErrNilSrcPtr{src: src})
	}

	if !src.Type().AssignableTo(dst.Type()) {
		convPanic(&ErrUnableAssignType{src.Type(), dst.Type()})
	}
	dst.Set(src)
}

type PtrConverter struct {
	elemEnc convFunc
}

func (pc *PtrConverter) conv(c *convState, src reflect.Value, dst reflect.Value, options Options, list ParamList) {
	//detect circle
	//fmt.Fprintln(os.Stderr,"->>",src.Type(),src)

	if dst.IsNil() {
		dst.Set(reflect.New(dst.Type().Elem()))
	}
	pc.elemEnc(c, src.Elem(), dst.Elem(), options, list)
	//when deepcopy is true
}

func newPtrConverter(srcType reflect.Type, dstType reflect.Type, options Options, list ParamList) convFunc {
	pc := new(PtrConverter)
	if options.DeepCopy {
		pc.elemEnc = newConv(srcType.Elem(), dstType.Elem(), options, list)
		return pc.conv
	} else {
		return func(c *convState, src reflect.Value, dst reflect.Value, options Options, list ParamList) {
			dst.Set(src)
		}
	}
}

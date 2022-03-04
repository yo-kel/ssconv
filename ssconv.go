package ssconv

import (
	"errors"
	"fmt"
	"github.com/mitchellh/hashstructure/v2"
	"os"
	"reflect"
	"strings"
	"sync"
)

type LocalRule struct {
}

type Options struct {
	DeepCopy   bool
	LocalRules []LocalRule
	hashCode   uint64
}

func (op *Options) hash() uint64 {
	if op.hashCode != 0 {
		return op.hashCode
	}
	var x int
	hashCode, err := hashstructure.Hash(x, hashstructure.FormatV2, nil)
	if err != nil {
		panic(err)
	}
	op.hashCode = hashCode
	return op.hashCode
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
		return newMapConverter(srcType, dstType, options, list)
	case reflect.Slice:
		return newSliceConverter(srcType, dstType, options, list)
	case reflect.Interface:
		return UnexpectedTypeConverter
	case reflect.Struct:
		return newStructConverter(srcType, dstType, options, list)

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

type field struct {
	name  string
	alias string
	tp    reflect.Type

	ignoreEmpty bool
	param       bool

	index     []int
	converter convFunc
}

type structField struct {
	List      []field
	NameIndex map[string][]int //use alias to index
}

type pairStructField struct {
	srcStruct structField
	dstStruct structField
}

type tagOptions []string

func parseTag(tag string) (name string, opts tagOptions) {
	res := strings.Split(tag, ",")
	if len(res) == 0 {
		return "", tagOptions{}
	} else if len(res) == 1 {
		return res[0], tagOptions{}
	}
	return res[0], res[1:]
}

type structFieldCacheKey struct {
	t        reflect.Type
	hashcode uint64
}

var structFieldCache sync.Map

func genStructFieldCacheKey(t reflect.Type, options Options) structFieldCacheKey {
	return structFieldCacheKey{
		t:        t,
		hashcode: options.hash(),
	}
}

func cachedStructField(t reflect.Type, options Options) structField {
	key := genStructFieldCacheKey(t, options)
	if f, ok := structFieldCache.Load(key); ok {
		return f.(structField)
	}
	f, _ := structFieldCache.LoadOrStore(key, extractStructFieldFields(t, options))
	return f.(structField)
}

func extractStructFieldFields(t reflect.Type, options Options) structField {
	//extract fields and anonymous struct fields(they are treated as same level)
	//at the time,any conv on unexported field is not supported

	current := []field{}
	nxt := []field{{tp: t}}

	fields := []field{}
	for len(nxt) > 0 {
		current, nxt = nxt, current[:0]
		for _, now := range current {
			for i := 0; i < now.tp.NumField(); i++ {

				ts := now.tp.Field(i)
				tft := ts.Type
				isUnexported := ts.PkgPath != ""

				//fmt.Fprintln(os.Stderr, "->>",ts.Name,ts.Anonymous)
				if isUnexported {
					if !(ts.Anonymous && tft.Kind() == reflect.Struct) {
						continue
					}
				}

				index := make([]int, len(now.index)+1)
				copy(index, now.index)
				index[len(now.index)] = i
				if !(tft.Kind() == reflect.Struct && ts.Anonymous) {
					tag := ts.Tag.Get("ssconv")

					alias, opts := parseTag(tag)

					if alias == "" {
						alias = ts.Name
					}

					var ignoreEmpty bool
					var param bool

					for _, opt := range opts {
						switch opt {
						case "ignoreEmpty":
							ignoreEmpty = true
						case "param":
							param = true
						}
					}

					f := field{
						name:        ts.Name,
						alias:       alias,
						tp:          ts.Type,
						ignoreEmpty: ignoreEmpty,
						param:       param,
						index:       index,
					}
					fields = append(fields, f)
					continue
				}
				nxt = append(nxt, field{name: ts.Name, index: index, tp: tft})
			}
		}
	}

	//converter of returned field is empty

	nameIndex := make(map[string][]int, len(fields))
	for i := range fields {
		f := &fields[i]
		nameIndex[f.name] = append(nameIndex[f.name], i)
		//fmt.Fprintln(os.Stderr, "->>",f.name,f.tp)
	}
	return structField{List: fields, NameIndex: nameIndex}
}

func extractPairStructFieldFields(srcType reflect.Type, dstType reflect.Type, options Options) (p pairStructField) {
	if srcType.Kind() != reflect.Struct {
		convPanic(ErrDstStructSrcNotStruct)
	}

	//options should be divided
	p.srcStruct = cachedStructField(srcType, options)
	p.dstStruct = cachedStructField(dstType, options)
	// cache  the field later

	for i := 0; i < len(p.dstStruct.List); i++ {
		f := &p.dstStruct.List[i]

		index, exist := p.srcStruct.NameIndex[f.name]
		if !exist {
			panic("field not exists")
		}
		if len(index) > 1 {
			panic("ambiguous field")
		}
		//fmt.Fprintln(os.Stderr, "->>",p.srcStruct.List[index[0]].name,f.name)
	}
	return p
}

type structConverter struct {
	pairStructField
}

func newStructConverter(srcType reflect.Type, dstType reflect.Type, options Options, list ParamList) convFunc {
	pair := extractPairStructFieldFields(srcType, dstType, options)

	structConv := structConverter{pairStructField: pair}
	return structConv.conv
}

func (s *structConverter) conv(c *convState, src reflect.Value, dst reflect.Value, options Options, list ParamList) {
	for i := 0; i < len(s.dstStruct.List); i++ {
		df := &s.dstStruct.List[i]
		dv := dst
		sIndex := s.srcStruct.NameIndex[df.name]
		sf := &s.srcStruct.List[sIndex[0]]
		sv := src

		for _, j := range df.index {
			dv = dv.Field(j)
		}

		for _, j := range sf.index {
			sv = sv.Field(j)
		}
		//fmt.Fprintln(os.Stderr, "->>",sv, dv)

		newConv(sv.Type(), dv.Type(), options, list)(c, sv, dv, options, list)
	}
}

type mapConverter struct {
	elemFunc convFunc
}

func newMapConverter(srcType reflect.Type, dstType reflect.Type, options Options, list ParamList) convFunc {
	if srcType.Kind() != reflect.Map {
		panic("src not map")
	}
	srcKey := srcType.Key()
	dstKey := dstType.Key()

	//map key must be the same
	if srcKey != dstKey {
		panic("different map key type")
	}

	srcElem := srcType.Elem()
	dstElem := dstType.Elem()

	var elemFunc convFunc
	if srcType != dstType {
		elemFunc = newConv(srcElem, dstElem, options, list)
	}
	mapConv := mapConverter{elemFunc: elemFunc}
	return mapConv.conv
}

func (m *mapConverter) conv(c *convState, src reflect.Value, dst reflect.Value, options Options, list ParamList) {

	if options.DeepCopy {
		dstKey := dst.Type().Key()
		dstElem := dst.Type().Elem()
		dstMap := reflect.MapOf(dstKey, dstElem)
		dst.Set(reflect.MakeMapWithSize(dstMap, len(src.MapKeys())))
		fmt.Fprintln(os.Stderr, "->>", src.MapKeys())
		for _, k := range src.MapKeys() {
			fmt.Fprintln(os.Stderr, "->>", k, src.MapIndex(k), dstElem)

			//map element is unaddressable
			//here we converter src to tmp value then to dst value
			if m.elemFunc == nil {
				dst.SetMapIndex(k, src.MapIndex(k))
			} else {
				tmpValue := reflect.New(dstElem)
				m.elemFunc(c, src.MapIndex(k), tmpValue, options, list)
				dst.SetMapIndex(k, tmpValue)
			}
		}
	} else {
		dst.Set(src)
	}
}

type sliceConverter struct {
	elemFunc convFunc
}

func (s *sliceConverter) conv(c *convState, src reflect.Value, dst reflect.Value, options Options, list ParamList) {
	if options.DeepCopy {
		srcElem := src.Type().Elem()
		elemSlice := reflect.SliceOf(srcElem)
		dst.Set(reflect.MakeSlice(elemSlice, src.Len(), src.Len()))
		//fmt.Fprintln(os.Stderr, "->>",src.Len())
		for i := 0; i < src.Len(); i++ {
			s.elemFunc(c, src.Index(i), dst.Index(i), options, list)
		}
	} else {
		dst.Set(src)
	}
}

func newSliceConverter(srcType reflect.Type, dstType reflect.Type, options Options, list ParamList) convFunc {
	if srcType.Kind() != reflect.Slice && srcType.Kind() != reflect.Array {
		panic("src not slice nor array")
	}

	srcElem := srcType.Elem()
	dstElem := dstType.Elem()

	sliceConv := sliceConverter{elemFunc: newConv(srcElem, dstElem, options, list)}
	return sliceConv.conv
}

package ssconv

import (
	"errors"
	"fmt"
	"github.com/barkimedes/go-deepcopy"
	"github.com/mitchellh/hashstructure/v2"
	"os"
	"reflect"
	"sort"
	"strings"
	"sync"
)

type CustomFunc func(data interface{}, param map[string]interface{}) (result interface{}, err error)

var customFuncType = reflect.TypeOf(func(data interface{}, param map[string]interface{}) (result interface{}, err error) { return nil, nil })

type LocalRule struct {
	field string
	// operation
}

type LocalRuleGroup struct {
	path string
	LocalRule
}

type Options struct {
	DeepCopy   bool
	LocalRules []*LocalRuleGroup
	hashCode   uint64
}

type OptionsProperty struct {
}

func (op *Options) hash() uint64 {
	if op == nil {
		return 0
	}
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

func (op *Options) redirect(path string) {
	if op == nil {
		return
	}
	for _, localRule := range op.LocalRules {
		if len(localRule.path) < len(path) || localRule.path[0:len(path)] != path {
			panic("invalid path")
		}
		tail := 0
		if len(localRule.path) > len(path) {
			if localRule.path[len(path)] != '.' {
				panic("invalid path")
			}
			tail = 1
		}
		localRule.path = localRule.path[len(path)+tail:]
	}
}

func (op *Options) split(s string) *Options {
	var splitRule []*LocalRuleGroup
	for _, localRule := range op.LocalRules {
		if len(localRule.path) > len(s) && localRule.path[0:len(s)] != s {
			splitRule = append(splitRule, localRule)
		}
	}
	res := new(Options)
	res.DeepCopy = op.DeepCopy
	cp, err := deepcopy.Anything(splitRule) // maybe change the library later
	if err != nil {
		panic(err)
	}
	res.LocalRules = cp.([]*LocalRuleGroup)
	return res
}

func (op *Options) SetDeepCode(deepCopy bool) *Options {
	op.DeepCopy = deepCopy
	return op
}

func (op *Options) IsEmpty() bool {
	return len(op.LocalRules) == 0 && op.DeepCopy == false
}

type ParamList struct {
}

type convState struct {
}

type convFunc func(c *convState, src reflect.Value, dst reflect.Value, list reflect.Value)

func Conv(src interface{}, dst interface{}, options *Options, list ParamList) (err error) {
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
	newConv(srcType, dstType, options)(c, srcValue, dstValue, reflect.ValueOf(list))
	return nil
}

func newConv(srcType, dstType reflect.Type, options *Options) convFunc {

	/*if dstType.Kind() != reflect.Ptr && dstType.Kind() != reflect.Map && dstType.Kind() != reflect.Slice {
		panic(ErrDstTypeNotReference)
	}*/

	//fmt.Fprintln(os.Stderr, dstType,srcType)

	//options that working in this level shou ld be divided into a new Options
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
		return newPtrConverter(srcType, dstType, options)
	case reflect.Map:
		return newMapConverter(srcType, dstType, options)
	case reflect.Slice:
		return newSliceConverter(srcType, dstType, options)
	case reflect.Interface:
		return UnexpectedTypeConverter
	case reflect.Struct:
		return newStructConverter(srcType, dstType, options)

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

func UnexpectedTypeConverter(c *convState, src reflect.Value, dst reflect.Value, list reflect.Value) {
	convPanic(&ErrUnexpectedType{dst.Type()})
}

func basicConverter(c *convState, src reflect.Value, dst reflect.Value, list reflect.Value) {
	//

	if src.Kind() == reflect.Ptr && src.IsNil() {
		convPanic(&ErrNilSrcPtr{src: src})
	}

	fmt.Fprintln(os.Stderr, src.Type())

	if !src.Type().AssignableTo(dst.Type()) {
		convPanic(&ErrUnableAssignType{src.Type(), dst.Type()})
	}
	dst.Set(src)
}

type PtrConverter struct {
	elemEnc convFunc
}

func (pc *PtrConverter) conv(c *convState, src reflect.Value, dst reflect.Value, list reflect.Value) {
	//detect circle
	//fmt.Fprintln(os.Stderr,"->>",src.Type(),src)

	if dst.IsNil() {
		dst.Set(reflect.New(dst.Type().Elem()))
	}
	pc.elemEnc(c, src.Elem(), dst.Elem(), list)
	//when deepcopy is true
}

func newPtrConverter(srcType reflect.Type, dstType reflect.Type, options *Options) convFunc {
	pc := new(PtrConverter)
	if options.DeepCopy {
		pc.elemEnc = newConv(srcType.Elem(), dstType.Elem(), options)
		return pc.conv
	} else {
		return func(c *convState, src reflect.Value, dst reflect.Value, list reflect.Value) {
			dst.Set(src)
		}
	}

}

type field struct {
	name  string
	alias string
	tp    reflect.Type

	hidden      bool
	ignoreEmpty bool

	param     bool
	paramName string

	customConv bool
	converter  reflect.Value

	index []int
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

func genStructFieldCacheKey(t reflect.Type, options *Options) structFieldCacheKey {
	return structFieldCacheKey{
		t:        t,
		hashcode: options.hash(),
	}
}

func cachedStructField(t reflect.Type, options *Options) structField {
	key := genStructFieldCacheKey(t, options)
	if f, ok := structFieldCache.Load(key); ok {
		return f.(structField)
	}

	f, _ := structFieldCache.LoadOrStore(key, extractStructFieldFields(t, options))
	return f.(structField)
}

func extractStructFieldFields(t reflect.Type, options *Options) structField {
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
					tag := ts.Tag.Get("conv")

					var ignoreEmpty bool
					var param bool
					var paramName string
					var hidden bool
					var customConv bool
					var method reflect.Value

					alias, opts := parseTag(tag)

					if alias == "" {
						alias = ts.Name
					} else if alias == "-" {
						alias = ""
						hidden = true
					}

					if len(opts) > 0 {
						switch opts[0] {
						case "param":
							if len(opts) > 2 {
								panic(errors.New("param: too many arguments"))
							}
							param = true
							paramName = opts[1]
						case "func":
							customConv = true

							funcName := alias
							if len(opts) >= 2 {
								funcName = opts[1]
							}

							m, exist := reflect.PtrTo(now.tp).MethodByName(funcName)
							if !exist {

								if funcName != alias {
									// if can not find method named funcName
									// try alias
									m, exist = reflect.PtrTo(now.tp).MethodByName(alias)
								}

								if !exist {
									panic(errors.New("cant find method"))
								}
							}

							//if method.Type()!=customFuncType{
							//	panic(errors.New("method "))
							//}

							method = m.Func

						default:
							for _, opt := range opts {
								switch opt {
								case "ignoreEmpty":
									ignoreEmpty = true
								}
							}
						}
					}

					f := field{
						name:  ts.Name,
						alias: alias,
						tp:    ts.Type,

						ignoreEmpty: ignoreEmpty,
						hidden:      hidden,

						param:     param,
						paramName: paramName,

						customConv: customConv,
						converter:  method,

						index: index,
					}
					fields = append(fields, f)
					continue
				}
				nxt = append(nxt, field{name: ts.Name, index: index, tp: tft})
			}
		}
	}

	sort.Slice(fields, func(i, j int) bool {
		if fields[i].hidden != fields[j].hidden {
			return !fields[i].hidden // fields[i].hidden<fields[j].hidden
		}
		return true
	})

	//converter of returned field is empty

	nameIndex := make(map[string][]int, len(fields))
	for i := range fields {
		f := &fields[i]
		nameIndex[f.alias] = append(nameIndex[f.alias], i)
		//fmt.Fprintln(os.Stderr, "->>",f.name,f.tp)
	}

	return structField{List: fields, NameIndex: nameIndex}
}

func extractPairStructFieldFields(srcType reflect.Type, dstType reflect.Type, options *Options) (p pairStructField) {
	if srcType.Kind() != reflect.Struct {
		convPanic(ErrDstStructSrcNotStruct)
	}

	// options should be divided
	p.srcStruct = cachedStructField(srcType, nil) //localRules can only be set on dst fields
	p.dstStruct = cachedStructField(dstType, options)
	// cache the field later
	for i := 0; i < len(p.dstStruct.List); i++ {
		f := &p.dstStruct.List[i]

		if f.hidden {
			break
		}

		index, exist := p.srcStruct.NameIndex[f.alias]
		if !exist {
			panic(fmt.Sprintf("field %s not exists", f.alias))
		}
		if len(index) > 1 {
			panic("ambiguous field")
		}
		if len(index) == 0 {
			panic("src field not exist")
		}
		if p.srcStruct.List[index[0]].hidden {
			panic("src field is hidden")
		}
		//fmt.Fprintln(os.Stderr, "->>",p.srcStruct.List[index[0]].name,f.name)
	}
	//
	if options != nil {
		//TODO delete unused field
	}
	return p
}

type structConverter struct {
	pairStructField
	options map[string]*Options
}

func newStructConverter(srcType reflect.Type, dstType reflect.Type, options *Options) convFunc {
	pair := extractPairStructFieldFields(srcType, dstType, options)
	options.redirect(srcType.Name())
	//fmt.Fprintln(os.Stderr,pair)

	sc := structConverter{pairStructField: pair, options: make(map[string]*Options)}
	//fmt.Fprintln(os.Stderr,sc.pairStructField)
	for i := 0; i < len(pair.dstStruct.List); i++ { // better way to do it ?
		df := &pair.dstStruct.List[i]
		if df.tp.Kind() != reflect.Struct {
			continue
		}
		_, exists := sc.options[df.alias]
		if exists {
			continue
		}

		sc.options[df.alias] = options.split(df.alias)

	}

	return sc.conv
}

var errorInterfaceType = reflect.TypeOf((*error)(nil)).Elem()

func (s *structConverter) conv(c *convState, src reflect.Value, dst reflect.Value, list reflect.Value) {
	for i := 0; i < len(s.dstStruct.List); i++ {
		df := &s.dstStruct.List[i]
		if df.hidden {
			break
		}

		// custom convertor
		if df.customConv {
			var in []reflect.Value
			in = append(in, dst.Addr(), src, list)
			ret := df.converter.Call(in)

			firstRet := -1
			for i, v := range ret {
				if v.Type() == errorInterfaceType {
					if !v.IsNil() {
						panic(v.Interface())
					}
					continue
				}
				if firstRet == -1 {
					firstRet = i
				}
			}
			if firstRet != -1 {
				dst.Set(ret[firstRet])
			}
		}

		// default convertor
		dv := dst
		sIndex := s.srcStruct.NameIndex[df.alias]
		//fmt.Println(i," ",sIndex[0])
		sf := &s.srcStruct.List[sIndex[0]]
		sv := src

		for _, j := range df.index {
			dv = dv.Field(j)
		}

		for _, j := range sf.index {
			sv = sv.Field(j)
		}
		//fmt.Fprintln(os.Stderr,df,dv,sv)

		if df.ignoreEmpty && sv.IsZero() {
			continue
		}

		if df.param {
			//TODO
		} else if df.customConv {
			//TODO
		} else {
			newConv(sv.Type(), dv.Type(), s.options[df.alias])(c, sv, dv, list)
		}
	}
}

type mapConverter struct {
	elemFunc convFunc
}

func newMapConverter(srcType reflect.Type, dstType reflect.Type, options *Options) convFunc {
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

	if !options.DeepCopy {
		return func(c *convState, src reflect.Value, dst reflect.Value, list reflect.Value) {
			dst.Set(src)
		}
	}
	var elemFunc convFunc
	//if srcType != dstType {
	elemFunc = newConv(srcElem, dstElem, options)
	//}

	mapConv := mapConverter{elemFunc: elemFunc}
	return mapConv.conv
}

func (m *mapConverter) conv(c *convState, src reflect.Value, dst reflect.Value, list reflect.Value) {
	dstKey := dst.Type().Key()
	dstElem := dst.Type().Elem()
	dstMap := reflect.MapOf(dstKey, dstElem)
	dst.Set(reflect.MakeMapWithSize(dstMap, len(src.MapKeys())))
	//fmt.Fprintln(os.Stderr, "->>", src.MapKeys())
	for _, k := range src.MapKeys() {

		//map element is unaddressable
		//here we converter src to tmp value then to dst value
		//fmt.Fprintln(os.Stderr, "->>", k, src.MapIndex(k), dstElem)
		if m.elemFunc == nil {
			dst.SetMapIndex(k, src.MapIndex(k))
		} else {
			tmpValue := reflect.New(dstElem).Elem()
			m.elemFunc(c, src.MapIndex(k), tmpValue, list)
			dst.SetMapIndex(k, tmpValue)
		}
	}
}

type sliceConverter struct {
	elemFunc convFunc
}

func (s *sliceConverter) conv(c *convState, src reflect.Value, dst reflect.Value, list reflect.Value) {
	srcElem := src.Type().Elem()
	elemSlice := reflect.SliceOf(srcElem)
	dst.Set(reflect.MakeSlice(elemSlice, src.Len(), src.Len()))
	//fmt.Fprintln(os.Stderr, "->>", src.Len())
	for i := 0; i < src.Len(); i++ {
		s.elemFunc(c, src.Index(i), dst.Index(i), list)
	}
}

func newSliceConverter(srcType reflect.Type, dstType reflect.Type, options *Options) convFunc {
	if srcType.Kind() != reflect.Slice && srcType.Kind() != reflect.Array {
		panic("src not slice nor array")
	}

	srcElem := srcType.Elem()
	dstElem := dstType.Elem()
	if !options.DeepCopy {
		return func(c *convState, src reflect.Value, dst reflect.Value, list reflect.Value) {
			dst.Set(src)
		}
	}
	sliceConv := sliceConverter{elemFunc: newConv(srcElem, dstElem, options)}
	return sliceConv.conv
}

package ssconv

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/mitchellh/hashstructure/v2"
	"github.com/mohae/deepcopy"
	"os"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"sync"
)

type CustomFunc func(data interface{}, param map[string]interface{}) (result interface{}, err error)

var customFuncType = reflect.TypeOf(func(data interface{}, param map[string]interface{}) (result interface{}, err error) { return nil, nil })

type LocalRule struct {
	field     string
	operation map[string]interface{}
}

func (rule *LocalRule) clone() *LocalRule {
	newrule := new(LocalRule)
	newrule.field = rule.field
	newrule.operation = deepcopy.Copy(rule.operation).(map[string]interface{})
	return newrule
}

type LocalRuleGroup struct {
	path  string
	rules []LocalRule
}

func NewLocalRuleGroup(path string) *LocalRuleGroup {
	ret := new(LocalRuleGroup)
	ret.path = path
	return ret
}

func (grp *LocalRuleGroup) AddRule(field string, operation map[string]interface{}) *LocalRuleGroup {
	grp.rules = append(grp.rules, LocalRule{
		field:     field,
		operation: operation,
	})
	return grp
}

func (grp *LocalRuleGroup) clone() *LocalRuleGroup {
	newgrp := new(LocalRuleGroup)
	newgrp.path = grp.path
	for _, rule := range grp.rules {
		newgrp.rules = append(newgrp.rules, *rule.clone())
	}
	return newgrp
}

type Options struct {
	DeepCopy   bool
	LocalRules []*LocalRuleGroup
	hashCode   uint64
}

func (op *Options) AddLocalRule(group *LocalRuleGroup) *Options {
	op.LocalRules = append(op.LocalRules, group)
	return op
}

func (op *Options) SetDeepcopy(dc bool) *Options {
	op.DeepCopy = dc
	return op
}

func (op *Options) clone() *Options {
	newop := new(Options)
	newop.DeepCopy = op.DeepCopy
	for _, grp := range op.LocalRules {
		newop.LocalRules = append(newop.LocalRules, grp.clone())
	}
	return newop
}

func (op *Options) hash() uint64 {
	if op == nil {
		return 0
	}
	if op.hashCode != 0 {
		return op.hashCode
	}
	hashCode, err := hashstructure.Hash(op, hashstructure.FormatV2, nil)
	if err != nil {
		panic(err)
	}
	op.hashCode = hashCode
	return op.hashCode
}

func (op *Options) redirect(path string) *Options {
	if op == nil {
		return op
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
	return op
}

func (op *Options) effect() *Options {
	if op == nil {
		return nil
	}
	var effectRule []*LocalRuleGroup
	for _, localRule := range op.LocalRules {
		if localRule.path == "" {
			effectRule = append(effectRule, localRule)
		}
	}

	ret := new(Options)
	ret.LocalRules = effectRule
	ret.DeepCopy = op.DeepCopy
	return ret
}

func (op *Options) split(s string) *Options {
	var splitRule []*LocalRuleGroup
	for _, localRule := range op.LocalRules {
		if len(localRule.path) > len(s) && localRule.path[0:len(s)] != s {
			splitRule = append(splitRule, localRule.clone())
		}
	}
	res := new(Options)
	res.DeepCopy = op.DeepCopy
	return res
}

func (op *Options) SetDeepCode(deepCopy bool) *Options {
	op.DeepCopy = deepCopy
	return op
}

func (op *Options) IsEmpty() bool {
	return len(op.LocalRules) == 0 && op.DeepCopy == false
}

type ParamList map[string]interface{}

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
	cacheConverter(srcType, dstType, options)(c, srcValue, dstValue, reflect.ValueOf(list))
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
	paramName reflect.Value

	customConv bool
	converter  reflect.Value

	index []int
}

type structField struct {
	List      []field
	NameIndex map[string]int //use alias to index
}

func (sf *structField) clone() *structField {
	newsf := new(structField)
	for _, f := range sf.List {
		nf := new(field)
		*nf = f
		newindex := make([]int, len(f.index))
		for i, v := range f.index {
			newindex[i] = v
		}
		nf.index = newindex
		newsf.List = append(newsf.List, *nf)
	}
	newsf.NameIndex = deepcopy.Copy(sf.NameIndex).(map[string]int)
	return newsf
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

	// change to cache
	_ret := extractStructFieldFields(t, nil)
	ret := _ret.clone()

	//fmt.Fprintln(os.Stderr,"->>",ret)
	//fmt.Fprintln(os.Stderr,_ret)
	if options != nil {
		opt := options.effect()
		for _, grp := range opt.LocalRules {
			for _, rule := range grp.rules {
				index, ok := ret.NameIndex[rule.field]
				if !ok {
					panic("localRule: cant find field")
				}
				for k, v := range rule.operation {
					f := &ret.List[index]
					switch k {
					case "func":
						if v == nil {
							f.customConv = false
						} else {
							f.customConv = true
							f.converter = reflect.ValueOf(v)
						}
					case "ignoreEmpty":
						ignoreEmpty, ok := v.(bool)
						if !ok {
							panic("localRule: cant find field")
						}
						f.ignoreEmpty = ignoreEmpty
					case "param":
						param, ok := v.(string)
						if !ok {
							panic("localRule: cant find field")
						}
						if param == "" {
							f.param = true
						} else {
							f.param = false
						}
						f.paramName = reflect.ValueOf(param)
					}
				}
			}
		}
	}

	f, _ := structFieldCache.LoadOrStore(key, *ret)

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
						paramName: reflect.ValueOf(paramName),

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

	nameIndex := make(map[string]int, len(fields))
	for i := range fields {
		f := &fields[i]

		//fmt.Fprintln(os.Stderr, "->>", t, f.name, f.tp)

		_, exist := nameIndex[f.alias]
		if exist {
			panic("duplicate field name")
		}
		nameIndex[f.alias] = i
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
	//fmt.Fprintln(os.Stderr,p.dstStruct)
	// cache the field later
	for i := 0; i < len(p.dstStruct.List); i++ {
		f := &p.dstStruct.List[i]

		if f.hidden {
			break
		}

		if f.customConv || f.param {
			continue
		}

		index, exist := p.srcStruct.NameIndex[f.alias]
		if !exist {
			panic(fmt.Sprintf("field %s not exists", f.alias))
		}
		if p.srcStruct.List[index].hidden {
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

var ConverterCache sync.Map

func cacheConverter(srcType reflect.Type, dstType reflect.Type, options *Options) convFunc {
	type pairKey struct {
		srcType    reflect.Type
		dstType    reflect.Type
		optionCode uint64
	}
	key := pairKey{
		srcType:    srcType,
		dstType:    dstType,
		optionCode: options.hash(),
	}
	//fmt.Fprint(os.Stderr, srcType, " ", srcType.PkgPath(), "->", options, "<-", key.optionCode)

	//key,err:=hashstructure.Hash(pairKey{
	//	srcType:    srcType,
	//	dstType:    dstType,
	//	optionCode: options.hash(),
	//},hashstructure.FormatV2,nil)
	//
	//if err!=nil{
	//	panic(err)
	//}

	if v, ok := ConverterCache.Load(key); ok {
		//fmt.Fprint(os.Stderr,srcType, " ",srcType.PkgPath(),")",v)
		return v.(convFunc)
	}
	v, _ := ConverterCache.LoadOrStore(key, newConv(srcType, dstType, options))
	return v.(convFunc)
}

func newStructConverter(srcType reflect.Type, dstType reflect.Type, options *Options) convFunc {
	pair := extractPairStructFieldFields(srcType, dstType, options)
	//fmt.Fprintln(os.Stderr,pair)

	sc := structConverter{pairStructField: pair, options: make(map[string]*Options)}
	//fmt.Fprintln(os.Stderr,sc.pairStructField)
	for i := 0; i < len(pair.dstStruct.List); i++ { // better way to do it ?
		df := &pair.dstStruct.List[i]
		if df.tp.Kind() != reflect.Struct {
			continue
		}
		_, exists := sc.options[df.alias]
		if exists { //TODO
			continue
		}

		sc.options[df.alias] = options.split(df.alias).redirect(df.alias)

	}

	return sc.conv
}

var errorInterfaceType = reflect.TypeOf((*error)(nil)).Elem()

func (s *structConverter) conv(c *convState, src reflect.Value, dst reflect.Value, list reflect.Value) {
	//fmt.Fprintln(os.Stderr,s.dstStruct.List)
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
			continue
		}

		//param convertor
		if df.param {

			v := list.MapIndex(df.paramName)
			if df.ignoreEmpty && !v.IsValid() {
				continue
			}

			dv := dst
			for _, j := range df.index {
				dv = dv.Field(j)
			}

			dv.Set(v)
		}

		// default convertor
		dv := dst
		sIndex := s.srcStruct.NameIndex[df.alias]
		//fmt.Println(i," ",sIndex[0])
		sf := &s.srcStruct.List[sIndex]
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
		newConv(sv.Type(), dv.Type(), s.options[df.alias])(c, sv, dv, list)
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

type typeJson map[string]interface{}

func getFunctionName(i reflect.Value) string {
	return runtime.FuncForPC(i.Pointer()).Name()
}

// ShowTypeJson return a string represent conversion property of given types and options in json,
// used for debugging
func ShowTypeJson(src, dst interface{}, options *Options) string {
	js := showTypeJson(reflect.TypeOf(src), reflect.TypeOf(dst), options)
	str, err := json.MarshalIndent(js, "", "    ")
	if err != nil {
		panic(err)
	}
	return string(str)
}

func showTypeJson(srcType reflect.Type, dstType reflect.Type, options *Options) typeJson {

	ret := make(typeJson)

	pair := extractPairStructFieldFields(srcType, dstType, options)

	for i := 0; i < len(pair.dstStruct.List); i++ {
		df := &pair.dstStruct.List[i]
		var fm typeJson
		fm = make(typeJson)
		//fm["type"]=df.tp.Name()
		if df.alias != df.name && df.alias != "" {
			fm["alias"] = df.alias
		}
		if df.hidden {
			fm["hidden"] = df.hidden
		}
		if df.ignoreEmpty {
			fm["ignoreEmpty"] = df.ignoreEmpty
		}
		if df.param {
			fm["param"] = df.paramName
		}
		if df.customConv {
			fm["func"] = getFunctionName(df.converter) + " " + df.converter.Type().String()
		}

		if df.param || df.customConv || df.hidden {
			if !df.hidden {
				ret[df.name] = fm
			}
			continue
		}

		fm["srcField"] = srcType.Name() + "." + pair.srcStruct.List[pair.srcStruct.NameIndex[df.alias]].name

		if df.tp.Kind() == reflect.Struct {

			dt := dstType
			sIndex := pair.srcStruct.NameIndex[df.alias]
			//fmt.Println(i," ",sIndex[0])
			sf := &pair.srcStruct.List[sIndex]
			st := srcType

			for _, j := range df.index {
				dt = dt.Field(j).Type
			}

			for _, j := range sf.index {
				st = st.Field(j).Type
			}

			if _, exist := ret["Subfield"]; !exist {
				ret["Subfield"] = make([]typeJson, 0)
			}
			ret["Subfield"] = append(ret["Subfield"].([]typeJson),
				showTypeJson(st, dt, options.split(df.alias).redirect(df.alias)))
		}
		ret[df.name] = fm
	}
	return typeJson{
		dstType.Name() + fmt.Sprintf("(hashcode:%d)", options.hash()): ret,
	}
}

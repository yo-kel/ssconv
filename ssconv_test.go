package ssconv

import (
	"fmt"
	"github.com/google/go-cmp/cmp"
	"os"
	"strconv"
	"testing"
)

var debug bool

func init() {
	debug, _ = strconv.ParseBool(os.Getenv("Debug"))
	debug = true
}

func debugOutput(a ...interface{}) {
	if !debug {
		return
	}
	fmt.Fprintln(os.Stderr, a...)

}

func TestBasicIntCon(t *testing.T) {
	var x, y int
	x = 1
	err := Conv(&x, &y, *new(Options), *new(ParamList))
	if err != nil {
		t.Error(err)
	}
	if !cmp.Equal(x, y) {
		t.Error()
	}
}

func TestBasicIntCon2(t *testing.T) {
	var x, y int
	x = 1
	err := Conv(&x, &y, *new(Options), *new(ParamList))
	if err != nil {
		t.Error(err)
	}
	if !cmp.Equal(x, y) {
		t.Error()
	}
}

func TestPtrCon(t *testing.T) {
	x := new(int)
	y := new(int)
	*x = 1
	*y = 2
	err := Conv(&x, &y, *new(Options).SetDeepCode(true), *new(ParamList))
	if err != nil {
		t.Error(err)
	}
	debugOutput(*x, x)
	debugOutput(*y, y)
	if !cmp.Equal(x, y) {
		t.Error()
	}
}

func TestPtrCon2(t *testing.T) {
	x := new(int)
	var y *int
	*x = 1
	err := Conv(&x, &y, *new(Options).SetDeepCode(true), *new(ParamList))
	if err != nil {
		t.Error(err)
	}
	debugOutput(*x, x)
	debugOutput(*y, y)
	if !cmp.Equal(x, y) {
		t.Error()
	}
}

func TestPtrCon3(t *testing.T) {
	x := new(int)
	y := new(int)
	*x = 1
	*y = 2
	err := Conv(&x, &y, *new(Options).SetDeepCode(false), *new(ParamList))
	if err != nil {
		t.Error(err)
	}
	debugOutput(*x, x)
	debugOutput(*y, y)
	if x != y {
		t.Error()
	}
}

func TestPtrCon4(t *testing.T) {
	x := new(int)
	var y *int
	*x = 1
	err := Conv(&x, &y, *new(Options).SetDeepCode(false), *new(ParamList))
	if err != nil {
		t.Error(err)
	}
	debugOutput(*x, x)
	debugOutput(*y, y)
	if x != y {
		t.Error()
	}
}

func TestPtrCon5(t *testing.T) {
	var x *int
	var y int
	err := Conv(&x, &y, *new(Options).SetDeepCode(true), *new(ParamList))
	debugOutput(y)

	if err == nil || err.Error() != "value of *int in src is nil" {
		t.Error()
	}
}

func TestPtrCon6(t *testing.T) {
	var x *int
	var y int
	err := Conv(&x, &y, *new(Options).SetDeepCode(false), *new(ParamList))
	debugOutput(y)

	if err == nil || err.Error() != "value of *int in src is nil" {
		t.Error()
	}
}

func TestStruct(t *testing.T) {
	type A struct {
		V1 int
		V2 float32
		V3 float64
	}
	a := A{V1: 1, V2: 2.1, V3: 3.1}

	type B struct {
		V1 int
		V2 float32
	}
	var b B
	_ = Conv(&a, &b, *new(Options).SetDeepCode(false), *new(ParamList))
	debugOutput(b.V1)
	debugOutput(b.V2)

	if b.V1 != 1 || b.V2 != 2.1 {
		t.Error()
	}
}

func TestStruct2(t *testing.T) {
	type A_A struct {
		V1 int
	}
	type A struct {
		A_A
		V2 float32
		V3 float64
	}
	a := A{A_A: A_A{1}, V2: 2.1, V3: 3.1}

	type B_B struct {
		V1 int
	}
	type B struct {
		B_B
		V2 float32
	}
	var b B
	_ = Conv(&a, &b, *new(Options).SetDeepCode(false), *new(ParamList))
	debugOutput(b.V1)
	debugOutput(b.V2)

	if b.V1 != 1 || b.V2 != 2.1 {
		t.Error()
	}
}
func TestStruct3(t *testing.T) {
	type A_A struct {
		V1 int
		V4 string
		V5 [3]int
	}
	type A struct {
		A_A
		V2 float32
		V3 float64
	}
	a := A{A_A: A_A{V1: 1, V4: "123", V5: [3]int{4, 5, 6}}, V2: 2.1, V3: 3.1}

	type B_B struct {
		V1 int
		V5 [3]int
	}
	type B struct {
		V2 float32
		B_B
	}
	var b B
	_ = Conv(&a, &b, *new(Options).SetDeepCode(false), *new(ParamList))
	debugOutput(b.V1)
	debugOutput(b.V2)

	if b.V1 != 1 || b.V2 != 2.1 || b.V5 != [3]int{4, 5, 6} {
		t.Error()
	}
}

func TestStruct4(t *testing.T) {
	type C struct {
		V4 string
		V5 int
	}
	type A struct {
		V1 int
		V2 float32
		V3 float64
		CC C
	}
	a := A{V1: 1, V2: 2.1, V3: 3, CC: C{V4: "123", V5: 2}}

	type B_B struct {
		V1 int
	}
	type B struct {
		V1 int
		V2 float32
		CC C
	}
	var b B
	_ = Conv(&a, &b, *new(Options).SetDeepCode(false), *new(ParamList))

	debugOutput(b)

	if b.V1 != 1 || b.V2 != 2.1 || b.CC.V4 != "123" || b.CC.V5 != 2 {
		t.Error()
	}
}

func TestSlice(t *testing.T) {
	x := []int{1, 2, 3, 4}
	var y []int

	_ = Conv(&x, &y, *new(Options).SetDeepCode(false), *new(ParamList))
	if !cmp.Equal(x, y) {
		t.Error()
	}
	y[0] = 5
	y[1] = 6
	if !cmp.Equal(x, y) {
		t.Error()
	}
}

func TestSlice2(t *testing.T) {
	x := []int{1, 2, 3, 4}
	var y []int

	_ = Conv(&x, &y, *new(Options).SetDeepCode(true), *new(ParamList))
	if !cmp.Equal(x, y) {
		t.Error()
	}
	y[0] = 5
	y[1] = 6
	if cmp.Equal(x, y) {
		t.Error()
	}
}

func TestSlice3(t *testing.T) {
	x := [4]int{1, 2, 3, 4}
	var y []int

	_ = Conv(&x, &y, *new(Options).SetDeepCode(true), *new(ParamList))
	if y[0] != 1 || y[1] != 2 || y[2] != 3 || y[3] != 4 {
		t.Error()
	}
	y[0] = 5
	y[1] = 6
	debugOutput(x)
	debugOutput(y)
	fmt.Println(cmp.Equal(x, y))
	if y[0] != 5 || y[1] != 6 || y[2] != 3 || y[3] != 4 || x[0] != 1 || x[1] != 2 || x[2] != 3 || x[3] != 4 {
		t.Error()
	}
}

func TestMap(t *testing.T) {
	x := map[string]int{"hello": 1, "world": 2}
	var y map[string]int
	_ = Conv(&x, &y, *new(Options).SetDeepCode(false), *new(ParamList))
	if !cmp.Equal(x, y) {
		t.Error()
	}
	y["hello"] = 3

	if !cmp.Equal(x, y) {
		t.Error()
	}
}

func TestMap1(t *testing.T) {
	x := map[string]int{"hello": 1, "world": 2}
	var y map[string]int
	_ = Conv(&x, &y, *new(Options).SetDeepCode(true), *new(ParamList))
	if !cmp.Equal(x, y) {
		t.Error()
	}
	y["hello"] = 3
	if cmp.Equal(x, y) {
		t.Error()
	}
	debugOutput(x)
	debugOutput(y)
}

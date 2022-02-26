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
	debug = false
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

package freak

import (
	"fmt"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"testing"
)

type tt testing.T

func TestCleanPath(t *testing.T) {
	testResult(t, cleanPath, "/").
		with("/").
		with("").
		with(" ").
		with(" / ").
		with("/////").
		run()

	testResult(t, cleanPath, "/foo/bar/").
		with("/foo/bar/").
		with("/foo/bar").
		with("foo/bar/").
		with("foo/bar").
		with("/foo/ bar/").
		with("/foo///bar/").
		with("////foo/////bar////").
		run()
}

func testResult(t *testing.T, fn interface{}, expect ...interface{}) *tester {
	var fnVal = reflect.ValueOf(fn)

	return &tester{
		t:      t,
		fn:     fnVal,
		fnName: filepath.Ext(runtime.FuncForPC(fnVal.Pointer()).Name())[1:],
		expect: expect,
		args:   nil,
	}
}

type tester struct {
	t      *testing.T
	fn     reflect.Value
	fnName string
	expect []interface{}
	args   [][]reflect.Value
}

func (t *tester) with(args ...interface{}) *tester {
	t.args = append(t.args, toValues(args))

	return t
}

func (t *tester) run() {
	t.t.Run(fmt.Sprintf("%s wants %q", t.fnName, t.expect[0]), func(tt *testing.T) {

		//		tt.Logf(fmt.Sprintf("%s wants: %s\n", t.fnName, "%q"), t.expect...)

		for i, args := range t.args {

			var res = toInterfaces(t.fn.Call(args))

			// compare values in `res` slice to values in `expect` slice
			if len(res) != len(t.expect) {
				tt.Errorf(t.fnName+" "+strconv.Itoa(i+1)+": want: %d results, got: %d", len(t.expect), len(res))
			}

			for j := range res {
				if res[j] != t.expect[j] {
					tt.Errorf(t.fnName+" "+strconv.Itoa(i+1)+": got: %q", res[j])
				}
			}
		}
	})

	//t.t.Log("----------------------------------------")
}

func toValues(itfs []interface{}) []reflect.Value {
	var v = make([]reflect.Value, len(itfs))
	for i := range itfs {
		v[i] = reflect.ValueOf(itfs[i])
	}
	return v
}

func toInterfaces(vals []reflect.Value) []interface{} {
	var itfs = make([]interface{}, len(vals))
	for i := range vals {
		itfs[i] = vals[i].Interface()
	}
	return itfs
}

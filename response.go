package freak

import (
	"net/http"
	"reflect"
)

type RouteData struct {
}

type Response struct {
	resp http.ResponseWriter
	req  *http.Request

	wrapperEndingFuncs []func(*Response)

	// This is for calling the provided callbacks via reflection.
	// It holds a circular reference to itself.
	thisAsValue reflect.Value

	buf []byte

	skipping bool
	halt     bool
}

func (r *Response) WriteBytes(b []byte) {
	r.buf = append(r.buf, escapeHTMLBytes(b)...)
}

func (r *Response) WriteString(s string) {
	r.buf = append(r.buf, escapeHTMLString(s)...)
}

func (r *Response) WriteStringNoEscape(s string) {
	r.buf = append(r.buf, s...)
}

func (r *Response) WriteBytesNoEscape(b []byte) {
	r.buf = append(r.buf, b...)
}

func (r *Response) DoComponent(c *component, dataI interface{}) {
	r.do(c, dataI)
}

type WrapperResponse struct {
	r *Response
}

func (wr *WrapperResponse) DoWrapper(w *wrapper, dataI interface{}) {
	var r = wr.r

	if w == nil || r.halt {
		return
	}

	// Originally provided by the calling component
	var temp = r.wrapperEndingFuncs
	r.wrapperEndingFuncs = nil

	r.do(&w.preContent, dataI)

	r.wrapperEndingFuncs = append(temp, func(r *Response) {
		r.do(&w.postContent, dataI)
	})
}

func (wr *WrapperResponse) SkipContent() {
	wr.r.skipping = true
}

func (r *Response) do(c *component, dataI interface{}) {
	if r.halt {
		return
	}

	var callArgs = [2]reflect.Value{r.thisAsValue, reflect.ValueOf(dataI)}

	var wrapperEndStack [][]func(*Response)
	var endStackIndex = -1

	if c.maxWrapperNesting != 0 {
		wrapperEndStack = getWrapEndingSliceStack(c.maxWrapperNesting)
		defer returnWrapEndingSliceStack(wrapperEndStack)
	}

	for i := 0; i < len(c.markers); i++ {
		var m = c.markers[i]

		r.buf = append(r.buf, m.htmlPrefix...)

		switch m.kind {
		case plainMarker:
			m.fn.Call(callArgs[:])
			if r.halt {
				return
			}

		case wrapperStartMarker:
			endStackIndex++

			if endStackIndex >= len(wrapperEndStack) {
				panic("unreachable")
			}

			var funcSlice = wrapperEndStack[endStackIndex]
			r.wrapperEndingFuncs = funcSlice[0:0:cap(funcSlice)]

			m.fn.Call(callArgs[:])

			if r.halt {
				return
			}

			// In case the end-funcs slice was grown beyond itw original capacity
			wrapperEndStack[endStackIndex] = r.wrapperEndingFuncs

			r.wrapperEndingFuncs = nil

			if !r.skipping {
				continue
			}
			r.skipping = false // reset

			// We're skipping to the ending, so set `i` and `m`, and fallthrough so
			// that we're not writing the htmlPrefix, since it's part of the content
			i = int(m.wrapperEndIndex)
			m = c.markers[i]
			fallthrough

		case wrapperEnd:
			if len(wrapperEndStack) == 0 {
				panic("unreachable")
			}

			var funcSlice = wrapperEndStack[endStackIndex]

			for i := len(funcSlice) - 1; i != -1 && !r.halt; i-- {
				funcSlice[i](r)
				if r.halt {
					return
				}
			}

			endStackIndex--

		default:
			panic("unreachable")
		}
	}

	if endStackIndex != -1 {
		panic("unreachable")
	}

	r.buf = append(r.buf, c.htmlTail...)
}

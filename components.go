package freak

import "reflect"

type component struct {
	markers           []*marker
	htmlTail          []byte
	maxWrapperNesting int
}

func Component(
	css CSS, htmlFlags HTMLCompressFlag, html HTML, markers ...Marker,
) *component {
	c, _ := processFuncs(
		css, compressHTML(htmlFlags, html), toInternalMarkers(markers), false,
	)

	return &c
}

type wrapper struct {
	preContent  component
	postContent component
}

func Wrapper(
	css CSS, htmlFlags HTMLCompressFlag, html HTML, markers ...Marker,
) *wrapper {

	c1, c2 := processFuncs(
		css, compressHTML(htmlFlags, html), toInternalMarkers(markers), true,
	)

	return &wrapper{
		preContent:  c1,
		postContent: c2,
	}
}

func (c *component) do(r *Response, dataI interface{}) {
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

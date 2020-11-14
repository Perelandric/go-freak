package freak

import "reflect"

type component struct {
	markers           []*marker
	htmlTail          []byte
	maxWrapperNesting int
}

func Component(css CSS, html HTML, markers ...Marker) *component {
	var c component

	contentMarkerIndex, _ := c.processFuncs(css, html, toInternalMarkers(markers))
	if contentMarkerIndex != -1 {
		panic("Only a Wrapper component may define a '${}' content marker")
	}
	return &c
}

type wrapper struct {
	preContent  component
	postContent component
}

func Wrapper(css CSS, html HTML, markers ...Marker) *wrapper {
	var c component
	contentMarkerIndex, firstHalfTail := c.processFuncs(css, html, toInternalMarkers(markers))

	if contentMarkerIndex == -1 {
		panic("A Wrapper must define a '${}' content marker")
	}

	return &wrapper{
		preContent: component{
			markers:           c.markers[0:contentMarkerIndex],
			htmlTail:          firstHalfTail,
			maxWrapperNesting: c.maxWrapperNesting,
		},
		postContent: component{
			markers:           c.markers[contentMarkerIndex:],
			htmlTail:          c.htmlTail,
			maxWrapperNesting: c.maxWrapperNesting,
		},
	}
}

func (c *component) do(r *Response, dataI interface{}) {
	if r.halt {
		return
	}

	var callArgs = []reflect.Value{reflect.ValueOf(r), reflect.ValueOf(dataI)}
	var wrapperEndStack [][]func(*Response)

	if c.maxWrapperNesting != 0 {
		// TODO: Use cached slices
		wrapperEndStack = make([][]func(*Response), 0, c.maxWrapperNesting)
	}

	for _, m := range c.markers {
		r.buf = append(r.buf, m.htmlPrefix...)

		switch m.kind {
		case plainMarker:
			m.fn.Call(callArgs)

		case wrapperStart:
			// TODO: Use a slice from a memory cache
			r.wrapperEndingFuncs = getRespEndingFuncSlice()
			m.fn.Call(callArgs)
			wrapperEndStack = append(wrapperEndStack, r.wrapperEndingFuncs)
			r.wrapperEndingFuncs = nil

		case wrapperEnd:
			if len(wrapperEndStack) == 0 {
				break // Internal error?
			}

			var lastIdx = len(wrapperEndStack) - 1
			var endingsForStart = wrapperEndStack[lastIdx]
			wrapperEndStack[lastIdx] = nil
			wrapperEndStack = wrapperEndStack[0:lastIdx:cap(wrapperEndStack)]

			for i := len(endingsForStart) - 1; i != -1 && !r.halt; i-- {
				endingsForStart[i](r)
			}

			returnRespEndingFuncSlice(endingsForStart)

			continue

		default:
			panic("unreachable")
		}

		if r.halt {
			return
		}
	}

	r.buf = append(r.buf, c.htmlTail...)
}

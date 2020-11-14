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
	var endStackIndex = -1

	if c.maxWrapperNesting != 0 {
		wrapperEndStack = getWrapEndingSliceStack(c.maxWrapperNesting)
		defer returnWrapEndingSliceStack(wrapperEndStack)
	}

	for _, m := range c.markers {
		r.buf = append(r.buf, m.htmlPrefix...)

		switch m.kind {
		case plainMarker:
			m.fn.Call(callArgs)

		case wrapperStart:
			endStackIndex++

			if endStackIndex >= len(wrapperEndStack) {
				// TODO: Internal error?

				// This should never really happen, since we know the maximum number
				// of nested wrappers that were defined for this component
			}

			r.wrapperEndingFuncs = wrapperEndStack[endStackIndex]

			m.fn.Call(callArgs)

			// In case the end-funcs slice was grown beyond itw original capacity
			wrapperEndStack[endStackIndex] = r.wrapperEndingFuncs

			r.wrapperEndingFuncs = nil

		case wrapperEnd:
			if len(wrapperEndStack) == 0 {
				break // TODO: Internal error?
			}

			var funcSlice = wrapperEndStack[endStackIndex]

			for i := len(funcSlice) - 1; i != -1 && !r.halt; i-- {
				funcSlice[i](r)
			}

			wrapperEndStack[endStackIndex] = funcSlice[0:cap(funcSlice)]
			endStackIndex--

			continue

		default:
			panic("unreachable")
		}

		if r.halt {
			return
		}
	}

	if endStackIndex != -1 {
		// TODO: Internal error?
	}

	r.buf = append(r.buf, c.htmlTail...)
}

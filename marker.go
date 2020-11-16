package freak

import (
	"fmt"
	"reflect"
	"regexp"
)

type CSS string
type HTML string

func Wrap(fn interface{}) func(*Response, interface{}) {
	var fVal = reflect.ValueOf(fn)

	return func(r *Response, data interface{}) {
		fVal.Call([]reflect.Value{
			reflect.ValueOf(&WrapperResponse{r: r}),
			reflect.ValueOf(data),
		})
	}
}

type Marker struct {
	Name string
	Func interface{}
}

type markerKind uint8

const (
	plainMarker = markerKind(iota)
	wrapperStart
	wrapperEnd
)

type marker struct {
	name       string
	fn         reflect.Value // func(r *freak.Response, d *exampleWrapperData)
	htmlPrefix []byte
	kind       markerKind
}

func toInternalMarkers(markers []Marker) []*marker {
	var res = make([]*marker, len(markers))

	for i, m := range markers {
		res[i] = &marker{
			name:       m.Name,
			fn:         reflect.ValueOf(m.Func), // TODO: I think we could partly convert this into a function
			htmlPrefix: nil,
			kind:       plainMarker,
		}
	}

	return res
}

var re = regexp.MustCompile(
	`(}})|(\${})|\${{([a-zA-Z][-_\w]*)\s|\${([a-zA-Z][-_\w]*)}`,
)

func (c *component) processFuncs(css CSS, html string, markers []*marker) (int, []byte) {
	var wrapperContentMarkerIndex = -1
	var wrapperTailBeforeContentMarker []byte

	var htmlPrefixStartIdx = 0
	var markerIndex = 0
	var currentWrapperNesting = 0

	// TODO: Need to verify balance of wrapper start and end points

	var m = re.FindAllStringSubmatchIndex(string(html), -1)

	for _, match := range m {

		var matchedSub = -1
		var subMatch string

		for i := 2; i < len(match); i += 2 {
			if match[i] != -1 {
				matchedSub = i / 2
				subMatch = html[match[i]:match[i+1]]
				break
			}
		}

		switch matchedSub {
		case 1: // Wrapper end '}}'
			var newMarker = &marker{
				fn:   reflect.ValueOf(nil),
				kind: wrapperEnd,
			}

			markers = append(append(append(
				make([]*marker, 0, len(markers)+1), markers[0:markerIndex]...), newMarker), markers[markerIndex:]...,
			)

			currentWrapperNesting--

		case 2: // Wrapper content '${}'
			if wrapperContentMarkerIndex != -1 {
				panic("Only one wrapper content marker '${}' is allowed")
			}

			wrapperContentMarkerIndex = markerIndex
			wrapperTailBeforeContentMarker = []byte(html[htmlPrefixStartIdx:match[0]])
			htmlPrefixStartIdx = match[1]
			continue

		case 3: // Wrapper start '${{foo'
			markers[markerIndex].kind = wrapperStart
			checkValid(subMatch, markers, markerIndex)

			currentWrapperNesting++

			if currentWrapperNesting > c.maxWrapperNesting {
				c.maxWrapperNesting = currentWrapperNesting
			}

		case 4: // Placeholder '${bar}'
			checkValid(subMatch, markers, markerIndex)

		default:
			panic("unreachable")
		}

		markers[markerIndex].htmlPrefix = []byte(html[htmlPrefixStartIdx:match[0]])
		htmlPrefixStartIdx = match[1]
		markerIndex++
	}

	c.htmlTail = []byte(html[htmlPrefixStartIdx:])

	if currentWrapperNesting != 0 {
		panic("Unbalanced Wrapper start and end points")
	}

	if markerIndex != len(markers) {
		panic("There must be an equal number of HTML markers and marker functions")
	}

	// Align memory of markers slice
	var aligned = make([]marker, len(markers), len(markers))
	c.markers = make([]*marker, len(markers), len(markers))
	for i, m := range markers {
		aligned[i] = *m
		c.markers[i] = &aligned[i]
	}

	return wrapperContentMarkerIndex, wrapperTailBeforeContentMarker
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func checkValid(name string, markers []*marker, markerIndex int) {
	if string(name) != markers[markerIndex].name {
		panic("Marker function names must be found in the same order as defined in HTML")
	}

	for _, m := range markers[markerIndex+1:] {
		if string(name) == m.name {
			panic(fmt.Sprintf("Marker names must be unique. Found multiple %q", name))
		}
	}

	var m = markers[markerIndex]
	if m.kind == wrapperStart {
		// TODO: Check that the function signature is correct

	} else if m.kind == plainMarker {
		// TODO: Check that the function signature is correct

	}
}

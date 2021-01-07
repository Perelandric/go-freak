package freak

import (
	"fmt"
	"reflect"
	"regexp"
)

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
	Name    string
	Static  string
	Dynamic interface{}
}

type markerKind uint8

const (
	plainMarker = markerKind(iota)
	wrapperStartMarker
	wrapperEnd
)

type marker struct {
	name            string
	callback        reflect.Value // func(r *freak.Response, d *exampleWrapperData)
	htmlPrefix      []byte
	wrapperEndIndex uint16
	kind            markerKind
}

var reMarkerParts = regexp.MustCompile(
	`(\${{}})|(}})|\${([a-zA-Z][-_\w]*){|\${([a-zA-Z][-_\w]*)}`,
)

func processFuncs(h *html, markers []Marker, c *component, wrapper *wrapper) {
	// Convert Marker slice to *marker slice
	var _markers = make([]*marker, len(markers))

	for i, m := range markers {
		_markers[i] = &marker{
			name:       m.Name,
			callback:   reflect.ValueOf(m.Dynamic),
			htmlPrefix: nil,
			kind:       0,
		}
		if m.Dynamic == nil {
			_markers[i].callback = reflect.ValueOf(0)
		}
	}

	h.compress()

	var html = h.out

	var isWrapper = wrapper != nil

	const unblanaced = "Unbalanced Wrapper start and end points"
	const onlyWrapperGetsContent = "Only a Wrapper component may define a '${{}}' content marker"
	const onlyOneContent = "Only one wrapper content marker '${{}}' is allowed"
	const wrapperMustDefineContent = "A Wrapper must define a '${{}}' content marker"
	const unequalMarkersAndFuncs = "Unequal number of HTML markers and marker functions"

	var markerIndexAfterContent = -1

	var htmlPrefixStartIdx = 0
	var markerIndex = 0
	var currentWrapperNesting = 0
	var maxWrapperNesting = 0

	var m = reMarkerParts.FindAllStringSubmatchIndex(html, -1)

	for _, match := range m {

		var matchedSub = -1
		var subMatch string

		// Discover which subgroup was matched for this match
		for i := 2; i < len(match); i += 2 {
			if match[i] != -1 {
				matchedSub = i / 2
				subMatch = html[match[i]:match[i+1]]
				break
			}
		}

		switch matchedSub {
		case 1: // Wrapper content '${{}}'
			if !isWrapper {
				fmt.Println("WARNING: Found '${{}}' in a non-Wrapper")
				continue
				//				panic(onlyWrapperGetsContent)
			}
			if markerIndexAfterContent != -1 {
				panic(onlyOneContent)
			}

			markerIndexAfterContent = markerIndex

			c.htmlTail = []byte(html[htmlPrefixStartIdx:match[0]])

			htmlPrefixStartIdx = match[1]

		case 2: // Wrapper end '}}'
			var newMarker = &marker{
				// callback never gets called, but the marker must not get removed
				callback: reflect.ValueOf(func() {}),
				kind:     wrapperEnd,
			}

			_markers = append(append(append(
				make([]*marker, 0, len(_markers)+1),
				_markers[0:markerIndex]...),
				newMarker),
				_markers[markerIndex:]...,
			)

			currentWrapperNesting--
			if currentWrapperNesting < 0 {
				panic(unblanaced)
			}

			giveEndIndexToMarkerStart(markerIndex, _markers)

			_markers[markerIndex].htmlPrefix = []byte(html[htmlPrefixStartIdx:match[0]])
			htmlPrefixStartIdx = match[1]
			markerIndex++

		case 3: // Wrapper start '${{foo'
			_markers[markerIndex].kind = wrapperStartMarker
			checkValid(subMatch, _markers, markerIndex)

			currentWrapperNesting++

			if currentWrapperNesting > maxWrapperNesting {
				maxWrapperNesting = currentWrapperNesting
			}

			_markers[markerIndex].htmlPrefix = []byte(
				html[htmlPrefixStartIdx:match[0]] + markers[markerIndex].Static,
			)
			htmlPrefixStartIdx = match[1]
			markerIndex++

		case 4: // Placeholder '${bar}'
			_markers[markerIndex].kind = plainMarker
			checkValid(subMatch, _markers, markerIndex)

			_markers[markerIndex].htmlPrefix = []byte(
				html[htmlPrefixStartIdx:match[0]] + markers[markerIndex].Static,
			)
			htmlPrefixStartIdx = match[1]
			markerIndex++

		default:
			panic("unreachable")
		}
	}

	if currentWrapperNesting != 0 {
		panic(unblanaced)
	}

	if markerIndex != len(_markers) {
		panic(unequalMarkersAndFuncs)
	}

	if isWrapper && markerIndexAfterContent == -1 {
		panic(wrapperMustDefineContent)
	}

	// Align memory of markers slice
	var aligned = make([]marker, len(_markers), len(_markers))
	c.markers = make([]*marker, len(_markers), len(_markers))
	for i, m := range _markers {
		aligned[i] = *m
		c.markers[i] = &aligned[i]
	}

	if !isWrapper {
		c.htmlTail = []byte(html[htmlPrefixStartIdx:])
		c.maxWrapperNesting = maxWrapperNesting
		removeMarkersWithNoCallback(c)
		stringCacheInsert(c)
		return
	}

	wrapper.preContent = component{
		markers:           c.markers[0:markerIndexAfterContent],
		htmlTail:          c.htmlTail,
		maxWrapperNesting: maxWrapperNesting,
	}
	wrapper.postContent = component{
		markers:           c.markers[markerIndexAfterContent:],
		htmlTail:          []byte(html[htmlPrefixStartIdx:]),
		maxWrapperNesting: maxWrapperNesting,
	}

	removeMarkersWithNoCallback(&wrapper.preContent)
	removeMarkersWithNoCallback(&wrapper.postContent)

	stringCacheInsert(&wrapper.preContent, &wrapper.postContent)

	return
}

func removeMarkersWithNoCallback(c *component) {
	for i := 0; i < len(c.markers); i++ {
		var m = c.markers[i]

		if m.callback.IsZero() == false {
			continue
		}

		if i+1 == len(c.markers) {
			c.htmlTail = append(m.htmlPrefix, c.htmlTail...)
			c.markers = c.markers[0 : len(c.markers)-1]

		} else {
			var nextM = c.markers[i+1]
			nextM.htmlPrefix = append(m.htmlPrefix, nextM.htmlPrefix...)

			c.markers = append(c.markers[0:i], c.markers[i+1:]...)
			i--
		}
	}

	if ln := len(c.markers); ln < cap(c.markers) {
		c.markers = c.markers[0:ln:ln]
	}
}

func giveEndIndexToMarkerStart(index int, markers []*marker) {
	for i := index - 1; i != -1; i-- {
		if markers[i].kind != wrapperStartMarker {
			continue
		}

		if markers[i].wrapperEndIndex != 0 {
			continue // Was the start of an earlier marker ending
		}

		markers[i].wrapperEndIndex = uint16(index)
		return
	}

	// The paired start should be found before the loop ends`
	panic("unreachable")
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
	if m.kind == wrapperStartMarker {
		// TODO: Check that the function signature is correct

	} else if m.kind == plainMarker {
		// TODO: Check that the function signature is correct

	}
}

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
	wrapperStartMarker
	wrapperEnd
)

type marker struct {
	name            string
	fn              reflect.Value // func(r *freak.Response, d *exampleWrapperData)
	htmlPrefix      []byte
	wrapperEndIndex uint16
	kind            markerKind
}

func toInternalMarkers(markers []Marker) []*marker {
	var res = make([]*marker, len(markers))

	for i, m := range markers {
		res[i] = &marker{
			name:       m.Name,
			fn:         reflect.ValueOf(m.Func),
			htmlPrefix: nil,
			kind:       0,
		}
	}

	return res
}

var re = regexp.MustCompile(
	`(}})|(\${})|\${([a-zA-Z][-_\w]*){|\${([a-zA-Z][-_\w]*)}`,
)

func processFuncs(
	css CSS, html string, markerFuncs []*marker, isWrapper bool,
) (component, component) {

	const unblanaced = "Unbalanced Wrapper start and end points"
	const onlyWrapperGetsContent = "Only a Wrapper component may define a '${}' content marker"
	const onlyOneContent = "Only one wrapper content marker '${}' is allowed"
	const wrapperMustDefineContent = "A Wrapper must define a '${}' content marker"
	const unequalMarkersAndFuncs = "Unequal number of HTML markers and marker functions"

	var markerIndexAfterContent = -1

	var htmlPrefixStartIdx = 0
	var markerIndex = 0
	var currentWrapperNesting = 0
	var maxWrapperNesting = 0

	var c1, c2 component

	var m = re.FindAllStringSubmatchIndex(string(html), -1)

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
		case 1: // Wrapper end '}}'
			var newMarker = &marker{
				fn:   reflect.ValueOf(nil),
				kind: wrapperEnd,
			}

			markerFuncs = append(append(append(
				make([]*marker, 0, len(markerFuncs)+1), markerFuncs[0:markerIndex]...), newMarker), markerFuncs[markerIndex:]...,
			)

			currentWrapperNesting--
			if currentWrapperNesting < 0 {
				panic(unblanaced)
			}

			giveEndIndexToMarkerStart(markerIndex, markerFuncs)

		case 2: // Wrapper content '${}'
			if !isWrapper {
				panic(onlyWrapperGetsContent)
			}
			if markerIndexAfterContent != -1 {
				panic(onlyOneContent)
			}

			markerIndexAfterContent = markerIndex

			c1.htmlTail = []byte(html[htmlPrefixStartIdx:match[0]])
			htmlPrefixStartIdx = match[1]
			continue

		case 3: // Wrapper start '${{foo'
			markerFuncs[markerIndex].kind = wrapperStartMarker
			checkValid(subMatch, markerFuncs, markerIndex)

			currentWrapperNesting++

			if currentWrapperNesting > maxWrapperNesting {
				maxWrapperNesting = currentWrapperNesting
			}

		case 4: // Placeholder '${bar}'
			markerFuncs[markerIndex].kind = plainMarker
			checkValid(subMatch, markerFuncs, markerIndex)

		default:
			panic("unreachable")
		}

		markerFuncs[markerIndex].htmlPrefix = []byte(html[htmlPrefixStartIdx:match[0]])
		htmlPrefixStartIdx = match[1]
		markerIndex++
	}

	if currentWrapperNesting != 0 {
		panic(unblanaced)
	}

	if markerIndex != len(markerFuncs) {
		panic(unequalMarkersAndFuncs)
	}

	if isWrapper && markerIndexAfterContent == -1 {
		panic(wrapperMustDefineContent)
	}

	// Align memory of markers slice
	var aligned = make([]marker, len(markerFuncs), len(markerFuncs))
	c1.markers = make([]*marker, len(markerFuncs), len(markerFuncs))
	for i, m := range markerFuncs {
		aligned[i] = *m
		c1.markers[i] = &aligned[i]
	}

	if !isWrapper {
		c1.htmlTail = []byte(html[htmlPrefixStartIdx:])
		c1.maxWrapperNesting = maxWrapperNesting
		return c1, c2 // c2 is ignored by the caller
	}

	return component{
			markers:           c1.markers[0:markerIndexAfterContent],
			htmlTail:          c1.htmlTail,
			maxWrapperNesting: maxWrapperNesting,
		},
		component{
			markers:           c1.markers[markerIndexAfterContent:],
			htmlTail:          []byte(html[htmlPrefixStartIdx:]),
			maxWrapperNesting: maxWrapperNesting,
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

package freak

type component struct {
	markers  []*marker
	htmlTail []byte
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
			markers:  c.markers[0:contentMarkerIndex],
			htmlTail: firstHalfTail,
		},
		postContent: component{
			markers:  c.markers[contentMarkerIndex:],
			htmlTail: c.htmlTail,
		},
	}
}

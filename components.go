package freak

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

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
		css, compressHTML(htmlFlags, html), toInternalMarkers(markers), nil,
	)

	return c
}

type wrapper struct {
	preContent  component
	postContent component
}

func Wrapper(
	css CSS, htmlFlags HTMLCompressFlag, html HTML, markers ...Marker,
) *wrapper {

	var w wrapper

	_, _ = processFuncs(
		css, compressHTML(htmlFlags, html), toInternalMarkers(markers), &w,
	)

	return &w
}

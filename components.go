package freak

type component struct {
	markers           []*marker
	htmlTail          []byte
	maxWrapperNesting int
}

func Component(
	css CSS, htmlFlags HTMLCompressFlag, html HTML, markers ...Marker,
) *component {
	var c component

	processFuncs(
		css, compressHTML(htmlFlags, html), toInternalMarkers(markers), &c, nil,
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

	var c component
	var w wrapper

	processFuncs(
		css, compressHTML(htmlFlags, html), toInternalMarkers(markers), &c, &w,
	)

	return &w
}

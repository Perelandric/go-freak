package freak

type component struct {
	markers           []*marker
	htmlTail          []byte
	maxWrapperNesting int
}

func Component(css, js string, html htmlCompress, markers ...Marker) *component {
	var c component
	processFuncs(css, js, html.compress(), markers, &c, nil)
	return &c
}

type wrapper struct {
	preContent  component
	postContent component
}

func Wrapper(css, js string, html htmlCompress, markers ...Marker) *wrapper {
	var c component
	var w wrapper
	processFuncs(css, js, html.compress(), markers, &c, &w)
	return &w
}

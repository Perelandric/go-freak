package freak

import (
	"path"
	"strings"
)

type SiteMapNode struct {
	children  []*SiteMapNode
	ancestors []*SiteMapNode

	path    string
	dirName string

	displayName string
	description string

	noSort bool
}

var rootPage SiteMapNode

func newSiteMapNode(pth string, route *Route) *SiteMapNode {
	if len(pth) > 1 && strings.HasSuffix(pth, "/") {
		pth = pth[0 : len(pth)-1] // strip away the trailing '/'
	}

	if len(route.DisplayName) == 0 {
		route.DisplayName = path.Base(pth)
	}
	if len(route.Description) == 0 {
		route.Description = route.DisplayName
	}

	var node = getSiteMapPosition(pth)

	node.path = pth
	node.displayName = route.DisplayName
	node.description = route.Description

	return node
}

func getSiteMapPosition(pth string) *SiteMapNode {
	if len(pth) <= 1 {
		return &rootPage
	}

	var parent = &rootPage
	var idx = 0

	for {
		var start = idx + 1
		idx = strings.IndexByte(pth[start:], '/')

		var foundSep = idx != -1
		idx += start

		var dirName = ""
		if foundSep {
			dirName = pth[start:idx]
		} else {
			dirName = pth[start:]
		}

		var found *SiteMapNode
		for _, ch := range parent.children {
			if ch.dirName == dirName {
				found = ch
			}
		}
		if found != nil && idx == -1 && len(found.children) == 0 {
			// Shouldn't arrive at a path termination more than once
			panic("internal error; duplicate paths")
		}

		if found == nil {
			found = &SiteMapNode{
				dirName:   dirName,
				ancestors: append(append([]*SiteMapNode{}, parent.ancestors...), parent),
			}
			parent.children = append(parent.children, found)
		}

		if !foundSep {
			return found
		}
		parent = found
	}
}

func (smn *SiteMapNode) Path() string {
	return smn.path
}
func (smn *SiteMapNode) Description() string {
	return smn.description
}
func (smn *SiteMapNode) DisplayName() string {
	return smn.displayName
}

func (smn *SiteMapNode) Parent() *SiteMapNode {
	if len(smn.ancestors) > 0 {
		return smn.ancestors[len(smn.ancestors)-1]
	}
	return nil
}

func (smn *SiteMapNode) EachChild(fn func(*SiteMapNode, int) bool) {
	for i, child := range smn.children {
		if fn(child, i) == false {
			break
		}
	}
}
func (smn *SiteMapNode) GetChild(idx int) *SiteMapNode {
	if idx < 0 {
		idx = len(smn.children) + idx
	}
	if 0 <= idx && idx < len(smn.children) {
		return smn.children[idx]
	}
	return nil
}
func (smn *SiteMapNode) LenChildren() int {
	return len(smn.children)
}

func (smn *SiteMapNode) EachAncestor(fn func(*SiteMapNode, int) bool) {
	for i, anc := range smn.ancestors {
		if fn(anc, i) == false {
			break
		}
	}
}
func (smn *SiteMapNode) GetAncestor(idx int) *SiteMapNode {
	if idx < 0 {
		idx = len(smn.ancestors) + idx
	}
	if 0 <= idx && idx < len(smn.ancestors) {
		return smn.ancestors[idx]
	}
	return nil
}
func (smn *SiteMapNode) LenAncestors() int {
	return len(smn.ancestors)
}

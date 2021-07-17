package freak

import (
	"sync"
)

var stringCache []*[]byte
var stringCacheMux sync.Mutex

func stringCacheInsert(comps ...*component) {
	stringCacheMux.Lock()
	defer stringCacheMux.Unlock()

	for _, comp := range comps {
		for _, m := range comp.markers {
			if len(m.htmlPrefix) == 0 {
				continue
			}

			stringCache = append(stringCache, &m.htmlPrefix)
		}

		if len(comp.htmlTail) == 0 {
			continue
		}
		stringCache = append(stringCache, &comp.htmlTail)
	}
}

var allHtmlBytes []byte

// Is run after all components have been initialized.
// Perhaps run again as new component are created if allowed during runtime.
func locateSubstrings() {
	stringCacheMux.Lock() // Locking is probably not necessary, but doesn't hurt.
	defer stringCacheMux.Unlock()

	// Calculate the total length needed for all static bytes
	var totalCap = 0
	for _, b := range stringCache {
		totalCap += len(*b)
	}

	// Create a single byte slice to hold all static bytes
	allHtmlBytes = make([]byte, 0, totalCap)

	// Copy all static bytes into the allHtmlBytes, and use the given
	// pointers to replace the existing slices with the correct subslice
	// of the monolithic slice
	for _, b := range stringCache {
		var currStartIdx = len(allHtmlBytes)

		allHtmlBytes = append(allHtmlBytes, (*b)...)

		*b = allHtmlBytes[currStartIdx:]
	}

	stringCache = nil
}

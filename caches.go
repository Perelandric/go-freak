package freak

import (
	"sync"
)

var stringCache []*[]byte
var stringCacheMux sync.Mutex

// Inserts the []byte pointer in length-order
func stringCacheInsert(bufs ...*[]byte) {
	stringCacheMux.Lock()
	defer stringCacheMux.Unlock()

	for _, buf := range bufs {
		var bufLen = len(*buf)
		if bufLen == 0 {
			return
		}

		stringCache = append(stringCache, buf)
	}

}

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
	var monolithic = make([]byte, 0, totalCap)

	// Copy all static bytes into the monolithic []byte, and use the given
	// pointers to replace the existing slices with the correct subslice
	// of the monolithic slice
	for _, b := range stringCache {
		var currStartIdx = len(monolithic)

		monolithic = append(monolithic, (*b)...)

		*b = monolithic[currStartIdx:]
	}
}

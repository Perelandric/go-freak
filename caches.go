package freak

import (
	"sort"
	"sync"
)

/*

TODO: No strings are being cached, so go back and enable this.

It reuses strings references to eliminate redundency in memory.

*/

var stringCache []*[]byte
var stringCacheMux sync.Mutex

// TODO: One thing I co

// Inserts the string pointer in length-order
func stringCacheInsert(buf *[]byte) {
	var bufLen = len(*buf)
	if bufLen == 0 {
		return
	}

	stringCacheMux.Lock()

	// TODO: Maintain length order (using binary search) and remove the sort below
	stringCache = append(stringCache, buf)

	stringCacheMux.Unlock()
}

// Is run after all Prototypes have been initialized.
// Perhaps run again as new Prototypes are created if allowed during runtime.
func locateSubstrings() {
	stringCacheMux.Lock() // Locking is probably not necessary, but doesn't hurt.
	defer stringCacheMux.Unlock()

	// TODO: Remove this sort when the above code gets updated.
	sort.Slice(stringCache, func(i, j int) bool {
		return len(*stringCache[i]) < len(*stringCache[j])
	})

	// TODO: After all static HTML data has been received, should I put it into
	//		one monolithic []byte, keep indices of where each one started, and then
	//		take subslices from that single []byte? That would align a good bit of
	//		memory.
	//  If I precalculate the sums of the slices, I can pre-allocate and then safely
	//   take pointers into the momo-slice.

	var totalLen = 0
	for _, b := range stringCache {
		totalLen += len(*b)
	}

	var monolithic = make([]byte, 0, totalLen)
	//	var indexPairs = make([]struct{
	//    start, end int, ptr *[]byte,
	//  }, 0, len(stringCache))

	for _, b := range stringCache {
		var currStartIdx = len(monolithic)

		monolithic = append(monolithic, (*b)...)

		var endIdx = len(monolithic)
		*b = monolithic[currStartIdx:endIdx]

		//		indexPairs = append(indexPairs, struct{
		// start, end int, ptr *[]byte
		// }{nextIdx, endIdx, b})
	}

	/*
		// Staring with the short strings (which are at the start), try to find a matching substring
		// in one of the strings that are equal length or longer (searching longest strings first).
		for i, last := 0, len(stringCache)-1; i < last; i++ {
			for j := last; j > i; j-- {
				var strI, strJ = *stringCache[i], *stringCache[j]
				var idx = bytes.Index(strJ, strI)

				if idx != -1 { // Update the `i` string with the matching substring at `j`
					*stringCache[i] = strJ[idx : idx+len(strI)]
					break
				}
			}
		}
	*/
}

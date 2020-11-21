package freak

import (
	"sort"
	"sync"
)

/*

Now that all static []byte are consolidated into a contiguous block of memory,
it would be good to try to find redundant patterns and consolidate those too.

To do this, we would need to maintain the original slices, or at least their
start and end points in the monolithic []byte. Then for each slice, search for
a match somewhere. When one is found, replace the []byte in the component using
the pointer, and then block out those bytes as not being available for matches.

This should have the effect of consolidating down the a smaller subset. I don't know
if this is the most computationally efficient, or if it will yield the smallest result,
but it should be better than nothing.

In the end, we should have a more compact block of memory.

Longest slices should be searched first.

I don't know if the "removed" slices should be removed immediately. Could be that there
will be some match that will start inside the more dense area, but extend out to where
an old, removed slice would have started.

Maybe just focus the searches first on the overlapping areas. To do this we would need
to keep track of those areas. Maybe also track how many overlaps there are, so that new
searches can always start in proximity of the most dense areas.

A separate []uint16 could maybe be used, where each index corresponds to a byte index
in the monolithic []byte, and it keeps track of how many overlapping slices there were
for each byte.

*/

var stringCache []*[]byte
var stringCacheMux sync.Mutex

// Inserts the []byte pointer in length-order
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

// Is run after all components have been initialized.
// Perhaps run again as new component are created if allowed during runtime.
func locateSubstrings() {
	stringCacheMux.Lock() // Locking is probably not necessary, but doesn't hurt.
	defer stringCacheMux.Unlock()

	// TODO: Remove this sort when the above code gets updated.
	sort.Slice(stringCache, func(i, j int) bool {
		return len(*stringCache[i]) < len(*stringCache[j])
	})

	// Calculate the total length needed for all static bytes
	var totalLen = 0
	for _, b := range stringCache {
		totalLen += len(*b)
	}

	// Create a single byte slice to hold all static bytes
	var monolithic = make([]byte, 0, totalLen)
	//	var indexPairs = make([]struct{
	//    start, end int, ptr *[]byte,
	//  }, 0, len(stringCache))

	// Copy all static bytes into the monolithic []byte, and use the given
	// pointers to replace the existing slices with the correct subslice
	// of the monolithic slice
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

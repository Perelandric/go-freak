package freak

import "sync"

// TODO: Eventually have different pools for different sizes (to some limit)
var wrapEndingSliceStackPool sync.Pool

func getWrapEndingSliceStack(expectedSize int) [][]func(*Response) {
	var s = wrapEndingSliceStackPool.Get().([][]func(*Response))

	if s == nil {
		s = make([][]func(*Response), expectedSize, expectedSize)

	} else {
		for cap(s) < expectedSize {
			s = append(s, make([]func(*Response), 0, 1))
		}

		if len(s) < expectedSize {
			s = s[0:cap(s):cap(s)]
		}
	}

	return s
}

const endingFuncMaxCap = 2
const endingSliceStackMaxCap = 2

func returnWrapEndingSliceStack(s [][]func(*Response)) {
	var maxCap = min(endingSliceStackMaxCap, cap(s))

	s = s[0:maxCap:maxCap]

	for i := range s {
		var maxFuncCap = min(endingFuncMaxCap, cap(s[i]))

		s[i] = s[i][0:maxFuncCap:maxFuncCap]

		for j := range s[i] {
			s[i][j] = nil
		}

		s[i] = s[i][0:0:maxFuncCap]
	}

	wrapEndingSliceStackPool.Put(s)
}

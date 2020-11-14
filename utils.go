package freak

import "sync"

// TODO: Eventually have different pools for different sizes (to some limit)
var wrapEndingFuncPool = sync.Pool{
	New: func() interface{} {
		return make([]func(*Response), 0, 1)
	},
}

func getWrapEndingFuncSlice() []func(*Response) {
	return wrapEndingFuncPool.Get().([]func(*Response))
}

const endingFuncMaxCap = 2

func returnWrapEndingFuncSlice(s []func(*Response)) {
	s = s[0:0:min(endingFuncMaxCap, cap(s))]
	for i := range s {
		s[i] = nil
	}
	wrapEndingFuncPool.Put(s)
}

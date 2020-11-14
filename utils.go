package freak

import "sync"

// TODO: Eventually have different pools for different sizes (to some limit)
var respEndingFuncPool = sync.Pool{
	New: func() interface{} {
		return make([]func(*Response), 0, 1)
	},
}

func getRespEndingFuncSlice() []func(*Response) {
	return respEndingFuncPool.Get().([]func(*Response))
}

const endingFuncMaxCap = 2

func returnRespEndingFuncSlice(s []func(*Response)) {
	s = s[0:0:min(endingFuncMaxCap, cap(s))]
	for i := range s {
		s[i] = nil
	}
	respEndingFuncPool.Put(s)
}

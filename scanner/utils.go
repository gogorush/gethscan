package scanner

import (
	"github.com/ethereum/go-ethereum/common"
)

// GetData get data[start:start+size] (won't out of index range),
// and right padding the bytes to size long
func GetData(data []byte, start, size uint64) []byte {
	length := uint64(len(data))
	if start > length {
		start = length
	}
	end := start + size
	if end > length {
		end = length
	}
	return common.RightPadBytes(data[start:end], int(size))
}

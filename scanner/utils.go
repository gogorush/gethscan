package scanner

import (
	"math/big"
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

// GetBigInt get big int from data[start:start+size]
func GetBigInt(data []byte, start, size uint64) *big.Int {
        length := uint64(len(data))
        if length <= start || size == 0 {
                return big.NewInt(0)
        }
        end := start + size
        if end > length {
                end = length
        }
        return new(big.Int).SetBytes(data[start:end])
}

package scanner

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/weijun-sh/gethscan/params"
	"github.com/weijun-sh/gethscan/tools"
	"github.com/anyswap/CrossChain-Bridge/rpc/client"

	"github.com/weijun-sh/gethscan/log"
)

var (
	adjustInterval   = 600 // seconds
	RPCClientTimeout = 5 // seconds
)

var (
	ErrRPCQueryError = errors.New("rpc query error")
	ErrNotFound      = errors.New("not found")
)

// AdjustGatewayOrder adjust gateway order once
func AdjustGatewayOrder() {
	// use block number as weight
	var weightedAPIs tools.WeightedStringSlice
	gateway := params.GetGatewayConfig()
	if gateway == nil {
		return
	}
	var maxHeight uint64
	length := len(gateway.APIAddress)
	for i := length; i > 0; i-- { // query in reverse order
		apiAddress := gateway.APIAddress[i-1]
		height, _ := GetLatestBlockNumberOf(apiAddress)
		weightedAPIs = weightedAPIs.Add(apiAddress, height)
		if height > maxHeight {
			maxHeight = height
		}
	}
        weightedAPIs.Reverse() // reverse as iter in reverse order in the above
        weightedAPIs = weightedAPIs.Sort()
        gateway.APIAddress = weightedAPIs.GetStrings()
        gateway.WeightedAPIs = weightedAPIs
	log.Info("adjust gateways", "result", gateway.WeightedAPIs)
}

func adjustGatewayOrder() {
        for adjustCount := 0; ; adjustCount++ {
                for i := 0; i < adjustInterval; i++ {
                        time.Sleep(1 * time.Second)
                }

                adjustGatewayOrder()

                if adjustCount%2 == 0 {
			gateway := params.GetGatewayConfig()
                        log.Info("adjust gateways", "result", gateway.WeightedAPIs)
                }
        }
}

func GetLatestBlockNumberOf(url string) (latest uint64, err error) {
        var result string
        err = client.RPCPostWithTimeout(RPCClientTimeout, &result, url, "eth_blockNumber")
        if err == nil {
                return GetUint64FromStr(result)
        }
        return 0, WrapRPCQueryError(err, "eth_blockNumber")
}

// GetUint64FromStr get uint64 from string.
func GetUint64FromStr(str string) (uint64, error) {
        res, ok := ParseUint64(str)
        if !ok {
                return 0, errors.New("invalid unsigned 64 bit integer: " + str)
        }
        return res, nil
}

// ParseUint64 parses s as an integer in decimal or hexadecimal syntax.
// Leading zeros are accepted. The empty string parses as zero.
func ParseUint64(s string) (uint64, bool) {
        if s == "" {
                return 0, true
        }
        if len(s) >= 2 && (s[:2] == "0x" || s[:2] == "0X") {
                v, err := strconv.ParseUint(s[2:], 16, 64)
                return v, err == nil
        }
        v, err := strconv.ParseUint(s, 10, 64)
        return v, err == nil
}

// FirstN first n chars of string
func FirstN(s string, n int) string {
        if len(s) > n {
                return s[:n] + "..."
        }
        return s
}

// WrapRPCQueryError wrap rpc error
func WrapRPCQueryError(err error, method string, params ...interface{}) error {
        if err == nil {
                return fmt.Errorf("call '%s %v' failed, err='%w'", method, params, ErrNotFound)
        }
        return fmt.Errorf("%w: call '%s %v' failed, err='%v'", ErrRPCQueryError, method, params, FirstN(err.Error(), 166))
}

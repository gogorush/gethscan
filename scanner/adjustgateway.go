package scanner

import (
	"errors"
	"time"

	"github.com/weijun-sh/gethscan/params"
	"github.com/weijun-sh/gethscan/tools"

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
		if length > 1 && i - 1 == 0 { // remove current
			weightedAPIs = weightedAPIs.Add(apiAddress, 0)
			continue
		}
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


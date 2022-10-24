package scanner

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/anyswap/CrossChain-Bridge/log"
	"github.com/weijun-sh/gethscan/params"
)

var (
        accountResourcePath = API_VERSION + "accounts/%s/resource/%s"
	//mpc = "0x06da2b6027d581ded49b2314fa43016079e0277a17060437236f8009550961d6" //testnet

	RouterContract = "0xd6d6372c8bde72a7ab825c00b9edd35e643fb94a61c55d9d94a9db3010098548"
	resourceType = RouterContract+"::Router::SwapOutEventHolder"
	aptosToken params.TokenConfig
)

func (scanner *ethSwapScanner) aptosInitClient() {
	scanner.client = &RestClient{
		Url:     scanner.gateway,
		Timeout: 10000,
	}
}

func init() {
	aptosToken.TxType = "routerswap"
	aptosToken.ChainID = "1000004280404"
	aptosToken.SwapServer = "http://172.26.142.154:28747/rpc"
	aptosToken.RouterContract = "0xd6d6372c8bde72a7ab825c00b9edd35e643fb94a61c55d9d94a9db3010098548"
}

func (scanner *ethSwapScanner) getCoinEventLog(target, resourceType, field_name string, start, limit int) {
	restClient := scanner.client
	resp := &[]CoinEvent{}
	err := restClient.GetEventsByEventHandle(resp, target, resourceType, field_name, start, limit)
	if err != nil {
		log.Fatal("GetEventsByEventHandle", "err", err)
	}
	json, err := json.Marshal(resp)
	if err != nil {
		log.Fatal("Marshal", "err", err)
	}
	println(string(json))
}

func (scanner *ethSwapScanner) getSwapinEventLog(target, resourceType, field_name string, start, limit int) {
	restClient := scanner.client
	resp := &[]SwapinEvent{}
	err := restClient.GetEventsByEventHandle(resp, target, resourceType, field_name, start, limit)
	if err != nil {
		log.Fatal("GetEventsByEventHandle", "err", err)
	}
	json, err := json.Marshal(resp)
	if err != nil {
		log.Fatal("Marshal", "err", err)
	}
	println(string(json))
}

func (scanner *ethSwapScanner) GetSwapoutEventLog(start, limit uint64) bool {
	return scanner.getSwapoutEventLog(RouterContract, resourceType, "events", start, limit)
}

func (scanner *ethSwapScanner) getSwapoutEventLog(target, resourceType, field_name string, start, limit uint64) bool {
	restClient := scanner.client
	resp := &[]SwapoutEvent{}
	err := restClient.GetEventsByEventHandle(resp, target, resourceType, field_name, int(start), int(limit))
	if err != nil {
		log.Warn("GetEventsByEventHandle", "err", err)
		return false
	}
	for _, event := range *resp {
		tx, err := restClient.GetTransactionByVersion(event.Version)
		if err != nil {
			log.Warn("GetEventsByEventHandle", "version", event.Version, "err", err)
			return false
		}
		fmt.Printf("version:%s, txhash:%s, status:%v type:%s\n", event.Version, tx.Hash, tx.Success, event.Type)
		// POST
		scanner.postRouterSwap(tx.Hash, 0, &aptosToken)
	}
	return true
}

func (scanner *ethSwapScanner) GetAccountResource() (uint64, error) {
	return scanner.getAccountResource(RouterContract, resourceType)
}

func (scanner *ethSwapScanner) getAccountResource(target, resourceType string) (uint64, error) {
	restClient := scanner.client
	resp := &ResourceData{}
	err := restClient.GetAccountResource(target, resourceType, resp)
	if err != nil {
		log.Warn("getAccountResource", "target", target, "err", err)
		return 0, err
	}
	//fmt.Printf("getAccountResource, target: %v, counter: %v\n", target, resp.Data.Events.Counter)
	counter, err := strconv.Atoi(resp.Data.Events.Counter)
	c := uint64(counter)
	return c, err
}


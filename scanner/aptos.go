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

	RouterContract = "0x976edbe162fe4746329ead6a9735b4a907a5fc25ab0662ee7b0caea0ed07e14d"
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
	aptosToken.ChainID = "1000004280405"
	aptosToken.SwapServer = "http://1.15.228.87:31408/rpc"
	aptosToken.RouterContract = "0x976edbe162fe4746329ead6a9735b4a907a5fc25ab0662ee7b0caea0ed07e14d"
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
		log.Fatal("GetEventsByEventHandle", "err", err)
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
	fmt.Printf("getAccountResource, target: %v, counter: %v\n", target, resp.Data.Events.Counter)
	counter, err := strconv.Atoi(resp.Data.Events.Counter)
	c := uint64(counter)
	return c, err
}


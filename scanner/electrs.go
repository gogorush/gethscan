package scanner

import (
	"fmt"

	"github.com/anyswap/CrossChain-Bridge/rpc/client"
	"github.com/anyswap/CrossChain-Bridge/tokens"
	"github.com/anyswap/CrossChain-Bridge/tokens/btc/electrs"
	"github.com/anyswap/CrossChain-Bridge/log"

	"github.com/weijun-sh/gethscan/mongodb"
	"github.com/weijun-sh/gethscan/params"
)

const (
	p2pkhType           = "p2pkh"
	p2shType            = "p2sh"
	opReturnType        = "op_return"
	ElctrsBlocks uint64 = 25 //elctrs API defined
)

// GetBlockTransactions impl
func (scanner *ethSwapScanner) GetBlockTransactions(blockHash string, startIndex uint32) ([]*electrs.ElectTx, error) {
	gateway := scanner.gateway
	apiAddress := gateway
	var err error
	var result []*electrs.ElectTx
	//for _, apiAddress := range gateway.APIAddress {
	url := apiAddress + "/block/" + blockHash + "/txs/" + fmt.Sprintf("%d", startIndex)
	err = client.RPCGet(&result, url)
	if err == nil {
		return result, nil
	}
	//}
	return nil, err
}

// GetBlockHash impl
func (scanner *ethSwapScanner) GetBlockHash(height uint64) (string, error) {
	gateway := scanner.gateway
	apiAddress := gateway
	var err error
	//for _, apiAddress := range gateway.APIAddress {
	url := apiAddress + "/block-height/" + fmt.Sprintf("%d", height)
	blockHash, err := client.RPCRawGet(url)
	if err == nil {
		return blockHash, nil
	}
	//}
	return "", err
}

// GetBlock impl
func (scanner *ethSwapScanner) GetBlock(blockHash string) (*electrs.ElectBlock, error) {
	gateway := scanner.gateway
	apiAddress := gateway
	var result electrs.ElectBlock
	var err error
	//for _, apiAddress := range gateway.APIAddress {
	url := apiAddress + "/block/" + blockHash
	err = client.RPCGet(&result, url)
	if err == nil {
		return &result, nil
	}
	//}
	return nil, err
}

// CheckSwapinTxType check swapin type
func (scanner *ethSwapScanner) CheckSwapinTxType(tx *electrs.ElectTx) (string, string, error) {
	for _, output := range tx.Vout {
		if output.ScriptpubkeyAddress == nil {
			continue
		}
		switch *output.ScriptpubkeyType {
		case p2shType, p2pkhType:
			serverRpc, bind, err := getP2shAddress(*output.ScriptpubkeyAddress)
			if err == nil {
				return serverRpc, bind, nil
			}
		}
	}
	return "", "", tokens.ErrWrongP2shBindAddress
}

func getP2shAddress(p2shAddress string) (string, string, error) {
	config := params.GetScanConfig()
	for _, token := range config.Tokens {
		dbname := token.DbName
		serverRpc := token.SwapServer
		bind, err := GetP2shAddressInfo(dbname, p2shAddress)
		if err == nil {
			return serverRpc, bind, nil
		}
	}
	return "", "", tokens.ErrWrongP2shBindAddress
}

// GetP2shAddressInfo api
func GetP2shAddressInfo(dbname, p2shAddress string) (string, error) {
	//log.Info("GetP2shAddressInfo", "dbname", dbname, "p2shAddress", p2shAddress)
        bindAddress, err := mongodb.FindP2shAddressInfo(dbname, p2shAddress)
        if err == nil {
		log.Info("GetP2shAddressInfo", "dbname", dbname, "p2shAddress", p2shAddress, "bind", bindAddress)
                return bindAddress, nil
        }
        return "", err
}


package scanner

import (
	"bytes"
	"errors"
	"encoding/json"
	"fmt"
	"net/http"
	"io/ioutil"
	"strings"
	"time"

	"github.com/anyswap/CrossChain-Bridge/log"

	"github.com/weijun-sh/gethscan/params"
	//"github.com/davecgh/go-spew/spew"
)
func (scanner *ethSwapScanner) loopGetLatestBlockNumber() uint64 {
	for { // retry until success
		slot, err := getLatestSlot(scanner.gateway)
		if err == nil {
			log.Info("get latest block number success", "height", slot)
			return slot
		}
		log.Warn("get latest block number failed", "err", err)
		time.Sleep(scanner.rpcInterval)
	}
}

type latestSlotConfig struct {
        Result uint64 `json:"result"`
}

func getLatestSlot(url string) (uint64, error) {
        data := "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"getSlot\",\"params\":[{}]}"
        slot := latestSlotConfig{}
        for i := 0; i < 1; i++ {
                reader := bytes.NewReader([]byte(data))
                resp, err := http.Post(url, "application/json", reader)
                if err != nil {
                        fmt.Println(err.Error())
                        return 0, err
                }
                defer resp.Body.Close()

                //spew.Printf("resp.Body: %#v\n", resp.Body)
                body, err := ioutil.ReadAll(resp.Body)
                //spew.Printf("body: %#v, string: %v\n", body, string(body))

                if err != nil {
                        fmt.Println(err.Error())
                        return 0, err
                }
                err = json.Unmarshal(body, &slot)
                if err != nil {
                        fmt.Println(err)
                        return 0, err
                }
		return slot.Result, nil
        }
        return 0, errors.New("GetLatestBlockNumber_XRP null")
}

type slotConfig struct {
        Result slotResultConfig `json:"result"`
}

type slotResultConfig struct {
	BlockTime uint64 `json:"blockTime"`
	Blockhash string `json:"blockhash"`
	Transactions []*slotTransactionsConfig `json:"transactions"`
}

type slotTransactionsConfig struct {
	Transaction *slotTransaction `json:"transaction"`
}

type slotTransaction struct {
	Message *slotMessage `json:"message"`
	Signatures []string `json:"signatures"`
}

type slotMessage struct {
	AccountKeys []string `json:"accountKeys"`
}

func (scanner *ethSwapScanner) loopGetBlock(height uint64) (block *slotResultConfig, err error) {
	for i := 0; i < 5; i++ { // with retry
		block, err = getBlock(scanner.gateway, height)
		if err == nil {
			return block, nil
		}
		log.Warn("get block failed", "height", height, "err", err)
		time.Sleep(scanner.rpcInterval)
	}
	return nil, err
}

func getBlock(url string, number uint64) (*slotResultConfig, error) {
        data := "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"getBlock\",\"params\":["+fmt.Sprintf("%v", number)+",{\"encoding\": \"json\",\"transactionDetails\":\"full\",\"rewards\":false,\"maxSupportedTransactionVersion\":0}]}"
        //data := "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"getBlock\",\"params\":[176264660,{\"encoding\": \"json\",\"transactionDetails\":\"full\",\"rewards\":false,\"maxSupportedTransactionVersion\":0}]}"
        slot := slotConfig{}
        for i := 0; i < 1; i++ {
                reader := bytes.NewReader([]byte(data))
                resp, err := http.Post(url, "application/json", reader)
		//fmt.Printf("1, resp: %v, err: %v\n", resp, err)
                if err != nil {
                        fmt.Println(err.Error())
                        return nil, err
                }
                defer resp.Body.Close()

                //spew.Printf("resp.Body: %#v\n", resp.Body)
                body, err := ioutil.ReadAll(resp.Body)
                //spew.Printf("body: %#v, string: %v\n", body, string(body))

                if err != nil {
                        fmt.Println(err.Error())
                        return nil, err
                }
                err = json.Unmarshal(body, &slot)
                if err != nil {
                        fmt.Println(err)
                        return nil, err
                }
		return &slot.Result, nil
        }
        return nil, errors.New("GetLatestBlockNumber_XRP null")
}

func (scanner *ethSwapScanner) verifyTransaction(height uint64, tx *slotTransaction, tokenCfg *params.TokenConfig) (verifyErr error) {
	txHash := tx.Signatures[0]
	switch {
	// router swap
	case tokenCfg.IsRouterSwapAll():
		log.Debug("verifyTransaction IsRouterSwapAll", "txHash", txHash)
		scanner.verifyAndPostRouterSwapTx(tx, tokenCfg)
		return nil

	default:
		verifyErr = nil
	}

	if verifyErr == nil {
		scanner.postBridgeSwap(txHash, tokenCfg)
	}
	return verifyErr
}

func (scanner *ethSwapScanner) verifyAndPostRouterSwapTx(tx *slotTransaction, tokenCfg *params.TokenConfig) {
	if tx == nil || tx.Message == nil || len(tx.Message.AccountKeys) == 0 || len(tx.Signatures) == 0 {
		return
	}
	if scanner.ignoreType(tokenCfg.TxType) {
		for _, hash := range tx.Signatures {
			scanner.postRouterSwap(hash, 0, tokenCfg)
		}
		return
	}
	keys := tx.Message.AccountKeys
	for _, account := range keys {
		if !strings.EqualFold(account, tokenCfg.RouterContract) {
			log.Debug("verifyAndPostRouterSwapTx", "address", account)
			continue
		}
		log.Info("verifyAndPostRouterSwapTx", "keys", keys)
		for _, hash := range tx.Signatures {
			scanner.postRouterSwap(hash, 0, tokenCfg)
		}
		break
	}
}

func (scanner *ethSwapScanner) scanTransaction(height uint64, tx *slotTransaction) {
	if tx == nil {
		return
	}

	txHash := tx.Signatures
	for _, tokenCfg := range params.GetScanConfig().Tokens {
		verifyErr := scanner.verifyTransaction(height, tx, tokenCfg)
		if verifyErr != nil {
			log.Debug("verify tx failed", "txHash", txHash, "err", verifyErr)
		}
	}
}


package scanner

import (
	"errors"
	"fmt"
	"time"

	"github.com/weijun-sh/gethscan/log"
	"github.com/weijun-sh/gethscan/params"
	"github.com/weijun-sh/gethscan/rpc/client"
)

const (
	url         = "https://graphql-api.mainnet.dandelion.link/"
	key         = "123"
	limit       = 100
	mpcAddr     = "addr1v92uwgj753t3ejlauk9rruxqz44u3uz527zxnsq9eyjn5qs3ut9p5"

	QueryMethod2 = "{transactions(limit: %d offset: %d order_by:{block: {number: asc}} where:{block:{ number :{_gte: %d _lte: %d}} metadata:{ key :{_eq:\"%s\"}} }){block{number hash} hash outputs{address txHash}}}"
	TIP_QL = "{ cardano { tip { number hash}}}"
)

func (scanner *ethSwapScanner) getTransactionsSafe(from, end uint64) error {
	log.Info("getTransactionsSafe", "from", from, "end", end)
	if from > end {
		log.Warn("getTransactionsSafe range error", "from", from, "end", end)
		return errors.New("getTransactionsSafe err: from big than end")
	}
	var offset uint64
	count := end - from
	quot := count / limit
	remain := count - quot * limit
	log.Info("getTransactionsSafe", "quot", quot, "remain", remain, "from", from, "end", end)
	for i := uint64(0); i < quot; i++ {
		offset += limit * i
		err := scanner.getTransactions(from, end, limit, offset)
		if err != nil {
			return err
		}
	}
	offset = quot * limit
	err := scanner.getTransactions(from, end, limit, offset)
	if err != nil {
		return err
	}
	return nil
}

func (scanner *ethSwapScanner) getTransactions(from, end, limit2, offset uint64) error {
	log.Info("getTransactions", "from", from, "end", end, "limit", limit2, "offset", offset)
	var res []string
	query := fmt.Sprintf(QueryMethod2, limit2, offset, from, end, key)
	result, err := scanner.getTransactionByMetadata(scanner.gateway, query)
	if err != nil {
		return err
	}
	for _, transaction := range result.Transactions {
		for _, utxo := range transaction.Outputs {
			scanner.scanTransaction(0, 0, &utxo)
		}
	}
	log.Info("fliter success finished", "res", res)
	return nil
}

func (scanner *ethSwapScanner) getTransactionByMetadata(url, params string) (*TransactionResult, error) {
	request := &client.Request{}
	request.Params = params
	request.ID = int(time.Now().UnixNano())
	request.Timeout = 60
	var result TransactionResult

	for i :=0; i < 3; i++ {
		err := client.CardanoPostRequest(url, request, &result)
		if err == nil {
			return &result, err
		}
		log.Warn("getTransactionByMetadata failed", "err", err, "url", url, "params", params)
		if i / 5 == 0 {
			scanner.getLatestClient()
		}
		time.Sleep(1 * time.Second)
	}
	return nil, errors.New("getTransactionByMetadata, error")
}

type TransactionResult struct {
	Transactions []Transaction `json:"transactions"`
}

type Transaction struct {
	Hash          string     `json:"hash"`
	Outputs       []Output   `json:"outputs"`
}

type Output struct {
	Address string `json:"address"`
	TxHash  string `json:"txHash"`
}

func (scanner *ethSwapScanner) loopGetLatestBlockNumber() (uint64, error) {
	for i := 0; i < 30; i++ {
		number, err := GetLatestBlockNumberOf(scanner.gateway)
		if err == nil {
			return number, nil
		}
		if i / 5 == 0 {
			scanner.getLatestClient()
		}
		time.Sleep(1 * time.Second)
	}
	return 0, errors.New("loopGetLatestBlockNumber error")
}

func GetLatestBlockNumberOf(url string) (uint64, error) {
	request := &client.Request{}
	request.Params = TIP_QL
	request.ID = int(time.Now().UnixNano())
	request.Timeout = 60
	var result TipResponse
	err := client.CardanoPostRequest(url, request, &result)
	if err == nil {
		log.Info("get latest block number success", "height", result.Cardano.Tip.BlockNumber)
		return result.Cardano.Tip.BlockNumber, nil
	}
	log.Warn("get latest block number failed", "err", err)
	return 0, errors.New("LoopGetLatestBlockNumber error")
}

type TipResponse struct {
	Cardano NodeTip `json:"cardano"`
}

type NodeTip struct {
	Tip Tip `json:"tip"`
}

type Tip struct {
	BlockNumber uint64 `json:"number"`
	Hash        string `json:"hash"`
}

func (scanner *ethSwapScanner) scanTransaction(height, index uint64, tx *Output) {
	if tx == nil {
		return
	}
	for _, tokenCfg := range params.GetScanConfig().Tokens {
		verifyErr := scanner.verifyTransaction(height, index, tx, tokenCfg)
		if verifyErr != nil {
			log.Debug("verify tx failed", "txHash", tx.TxHash, "err", verifyErr)
		}
	}
}

func (scanner *ethSwapScanner) verifyTransaction(height, index uint64, tx *Output, tokenCfg *params.TokenConfig) (verifyErr error) {
	isAcceptToAddr := scanner.checkTxToAddress(tx, tokenCfg)
	if !isAcceptToAddr {
		log.Debug("verifyTransaction !isAcceptToAddr return", "txHash", tx.TxHash)
		return nil
	}

	txHash := tx.TxHash

	switch {
	// router swap
	case tokenCfg.IsRouterSwapAll():
		log.Debug("verifyTransaction IsRouterSwapAll", "txHash", txHash)
		scanner.verifyAndPostRouterSwapTx(tx, tokenCfg)
		return nil
	}

	return nil // TODO
}

func (scanner *ethSwapScanner) checkTxToAddress(tx *Output, tokenCfg *params.TokenConfig) (isAcceptToAddr bool) {
	if tx.Address == tokenCfg.RouterContract {
		return true
		//log.Info("fliter success", "hash", transaction.Hash)
	}
	return false
}

func (scanner *ethSwapScanner) verifyAndPostRouterSwapTx(tx *Output, tokenCfg *params.TokenConfig) {
	if scanner.ignoreType(tokenCfg.TxType) {
		scanner.postRouterSwap(tx.TxHash, "0", 0, tokenCfg)
		return
	}
	scanner.postRouterSwap(tx.TxHash, "0", 0, tokenCfg)
}


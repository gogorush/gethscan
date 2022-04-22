package scanner

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/weijun-sh/gethscan/params"

	"github.com/jowenshaw/gethclient/types/ethereum"
	"github.com/jowenshaw/gethclient/common"
        "github.com/jowenshaw/gethclient/types"
        "github.com/anyswap/CrossChain-Bridge/log"
)

var (
	RouterAnycallTopic          = common.HexToHash("0x9ca1de98ebed0a9c38ace93d3ca529edacbbe199cf1b6f0f416ae9b724d4a81c")

	tokenSwap map[string]*params.TokenConfig = make(map[string]*params.TokenConfig, 0)
	prefixSwapRouterAnycall = "routeranycall"

	fqSwapRouterAnycall ethereum.FilterQuery
	filterLogsRouterAnycallChan chan types.Log
)

func (scanner *ethSwapScanner) subscribeSwap(fq ethereum.FilterQuery, ch chan types.Log) {
        ctx := context.Background()
        sub := scanner.LoopSubscribe(ctx, fq, ch)
        defer sub.Unsubscribe()

        for {// check
                select {
                case err := <-sub.Err():
                        log.Info("Subscribe swap error restart", "error", err)
                        sub.Unsubscribe()
                        sub = scanner.LoopSubscribe(ctx, fq, ch)
                }
        }
}

func (scanner *ethSwapScanner) subscribe() {
        log.Info("start subscribe")
        if len(fqSwapRouterAnycall.Addresses) > 0 {
                scanner.subscribeSwap(fqSwapRouterAnycall, filterLogsRouterAnycallChan)
        }
}

func (scanner *ethSwapScanner) LoopSubscribe(ctx context.Context, fq ethereum.FilterQuery, ch chan types.Log) ethereum.Subscription {
        for {
                sub, err := scanner.client.SubscribeFilterLogs(ctx, fq, ch)
                if err == nil {
                        return sub
                }
                log.Info("Subscribe logs failed, retry in 1 second", "error", err)
                time.Sleep(time.Second * 1)
        }
}

func (scanner *ethSwapScanner) initGetlogs() {
        initFilterChan()
        defer closeFilterChain()

        initFilerLogs()

	if len(fqSwapRouterAnycall.Addresses) > 0 {
		go scanner.loopFilterChain()
		go scanner.subscribe()
	}
	select {}
}

func initFilterChan() {
        filterLogsRouterAnycallChan = make(chan types.Log, 128)
}

func closeFilterChain() {
        close(filterLogsRouterAnycallChan)
}

func initFilerLogs() {
        tokenSwap = make(map[string]*params.TokenConfig, 0)
        var tokenRouterAnycallAddresses = make([]common.Address, 0)
        tokens := params.GetScanConfig().Tokens
        for _, tokenCfg := range tokens {
                switch {
                // router anycall swap
                case tokenCfg.IsRouterAnycallSwap():
                        key := strings.ToLower(fmt.Sprintf("%v-%v", prefixSwapRouterAnycall, common.HexToAddress(tokenCfg.RouterContract)))
                        addTokenSwap(key, tokenCfg)
                        tokenRouterAnycallAddresses = append(tokenRouterAnycallAddresses, common.HexToAddress(tokenCfg.RouterContract))
		}
        }
        //router anycall
        if len(tokenRouterAnycallAddresses) > 0 {
                topicsAnycall := make([][]common.Hash, 0)
                topicsAnycall = append(topicsAnycall, []common.Hash{RouterAnycallTopic})
                fqSwapRouterAnycall.Addresses = tokenRouterAnycallAddresses
                fqSwapRouterAnycall.Topics = topicsAnycall
        }
}

func addTokenSwap(key string, tokenCfg *params.TokenConfig) {
        if tokenSwap[key] != nil {
                log.Fatal("addTokenSwap duplicate", "key", key)
        }
        if tokenCfg == nil {
                log.Fatal("tokenCfg is nil")
        }
        tokenSwap[key] = tokenCfg
}

func (scanner *ethSwapScanner) loopFilterChain() {
        for {
                select {
                case rlog := <-filterLogsRouterAnycallChan:
                        txhash := rlog.TxHash.String()
                        logIndex := scanner.getIndexPosition(rlog.TxHash, rlog.Index)
                        //log.Info("Find event filterLogsRouterAnycallChan", "txhash", txhash, "logIndex", logIndex)
                        key := strings.ToLower(fmt.Sprintf("%v-%v", prefixSwapRouterAnycall, rlog.Address))
                        token := tokenSwap[key]
                        if token == nil {
                                log.Debug("Find event filterLogsRouterAnycallChan", "txhash", txhash, "key not config", key)
                                continue
                        }
                        scanner.postRouterSwap(txhash, logIndex, token)
                }
        }
}

func (scanner *ethSwapScanner) getIndexPosition(txhash common.Hash, index uint) int {
        r, err := scanner.loopGetTxReceipt(txhash)
        if err != nil {
                log.Warn("get tx receipt error", "txHash", txhash, "err", err)
                return 0
        }
        for i, log := range r.Logs {
                if log.Index == index {
                        return i
                }
        }
        return 0
}

func (scanner *ethSwapScanner) getLogsSwapRouterAnycall(from, to uint64, cache bool) {
        if len(fqSwapRouterAnycall.Addresses) > 0 {
                scanner.filterLogs(from, to, fqSwapRouterAnycall, filterLogsRouterAnycallChan, cache)
        }
}

func (scanner *ethSwapScanner) getLogs(from, to uint64, cache bool) {
        scanner.getLogsSwapRouterAnycall(from, to, cache)
}

func (scanner *ethSwapScanner) filterLogs(from, to uint64, fq ethereum.FilterQuery, ch chan types.Log, cache bool) {
        ctx := context.Background()
        fq.FromBlock = big.NewInt(int64(from))
        fq.ToBlock = big.NewInt(int64(to))
        for i := 0; i < scanner.rpcRetryCount; i++ {
                logs, err := scanner.client.FilterLogs(ctx, fq)
                if err == nil {
                        //log.Info("filterLogs", "block from", from, "to", to, "logs", logs)
                        for _, log := range logs {
                                blockhash := log.BlockHash.String()
                                if cache && cachedBlocks.isScanned(blockhash) {
                                        continue
                                }
                                cachedBlocks.addBlock(blockhash)
                                ch <- log
                        }
                        //log.Info("filterLogs success", "block", height)
                        return
                }
                log.Warn("filterLogs retry in 200 millisecond", "error", err, "block from", from, "to", to)
                time.Sleep(200 * time.Millisecond)
        }
        log.Warn("filterLogs failed", "block from", from, "to", to)
}

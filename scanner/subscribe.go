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
	// router
	RouterAnySwapOutTopic                  = common.HexToHash("0x97116cf6cd4f6412bb47914d6db18da9e16ab2142f543b86e207c24fbd16b23a")
	RouterAnySwapOutTopic2                 = common.HexToHash("0x409e0ad946b19f77602d6cf11d59e1796ddaa4828159a0b4fb7fa2ff6b161b79")
	RouterAnySwapTradeTokensForTokensTopic = common.HexToHash("0xfea6abdf4fd32f20966dff7619354cd82cd43dc78a3bee479f04c74dbfc585b3")
	RouterAnySwapTradeTokensForNativeTopic = common.HexToHash("0x278277e0209c347189add7bd92411973b5f6b8644f7ac62ea1be984ce993f8f4")
        RouterCrossDexTopic                    = common.HexToHash("0x8e7e5695fff09074d4c7d6c71615fd382427677f75f460c522357233f3bd3ec3")

	RouterAnySwapOutV7Topic = common.HexToHash("0x0d969ae475ff6fcaf0dcfa760d4d8607244e8d95e9bf426f8d5d69f9a3e525af")
	RouterAnySwapOutAndCallV7Topic = common.HexToHash("0x968608314ec29f6fd1a9f6ef9e96247a4da1a683917569706e2d2b60ca7c0a6d")

	// router nft
	RouterNFT721SwapOutTopic       = common.HexToHash("0x0d45b0b9f5add3e1bb841982f1fa9303628b0b619b000cb1f9f1c3903329a4c7")
	RouterNFT1155SwapOutTopic      = common.HexToHash("0x5058b8684cf36ffd9f66bc623fbc617a44dd65cf2273306d03d3104af0995cb0")
	RouterNFT1155SwapOutBatchTopic = common.HexToHash("0xaa428a5ab688b49b415401782c170d216b33b15711d30cf69482f570eca8db38")

	// anycall
	RouterAnycallTopic          = common.HexToHash("0x9ca1de98ebed0a9c38ace93d3ca529edacbbe199cf1b6f0f416ae9b724d4a81c")
	RouterAnycallTransferSwapOutTopic = common.HexToHash("0xcaac11c45e5fdb5c513e20ac229a3f9f99143580b5eb08d0fecbdd5ae8c81ef5")

	RouterAnycallV6Topic        = common.HexToHash("0xa17aef042e1a5dd2b8e68f0d0d92f9a6a0b35dc25be1d12c0cb3135bfd8951c9")
	RouterAnycallV7Topic = common.HexToHash("0x17dac14bf31c4070ebb2dc182fc25ae5df58f14162a7f24a65b103e22385af0d")
	RouterAnycallV7Topic2 = common.HexToHash("0x36850177870d3e3dca07a29dcdc3994356392b81c60f537c1696468b1a01e61d")
)

var (
	tokenSwap map[string]*params.TokenConfig = make(map[string]*params.TokenConfig, 0)
        prefixSwapRouter = "router"
        prefixSwapRouterNFT = "routernft"
	prefixSwapRouterAnycall = "routeranycall"

        fqSwapRouter        ethereum.FilterQuery
        fqSwapRouterNFT     ethereum.FilterQuery
	fqSwapRouterAnycall ethereum.FilterQuery

	filterLogsRouterChan        chan types.Log
        filterLogsRouterNFTChan     chan types.Log
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
	if len(fqSwapRouter.Addresses) > 0 {
                go scanner.subscribeSwap(fqSwapRouter, filterLogsRouterChan)
        }
        if len(fqSwapRouterNFT.Addresses) > 0 {
                go scanner.subscribeSwap(fqSwapRouterNFT, filterLogsRouterNFTChan)
        }
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

	go scanner.loopFilterChain()
	select {}
}

func initFilterChan() {
        filterLogsRouterChan = make(chan types.Log, 128)
        filterLogsRouterNFTChan = make(chan types.Log, 128)
        filterLogsRouterAnycallChan = make(chan types.Log, 128)
}

func closeFilterChain() {
        close(filterLogsRouterChan)
        close(filterLogsRouterNFTChan)
        close(filterLogsRouterAnycallChan)
}

func initFilerLogs() {
        tokenSwap = make(map[string]*params.TokenConfig, 0)
	var tokenRouterAddresses = make([]common.Address, 0)
        var tokenRouterAnycallAddresses = make([]common.Address, 0)
        var tokenRouterNFTAddresses = make([]common.Address, 0)
        tokens := params.GetScanConfig().Tokens
        for _, tokenCfg := range tokens {
                switch {
                // router swap
                case tokenCfg.IsRouterSwap():
			log.Debug("initFilerLogs", "IsRouterSwap", tokenCfg.RouterContract)
                        key := strings.ToLower(fmt.Sprintf("%v-%v", prefixSwapRouter, tokenCfg.RouterContract))
                        addTokenSwap(key, tokenCfg)
                        tokenRouterAddresses = append(tokenRouterAddresses, common.HexToAddress(tokenCfg.RouterContract))

                // router NFT
                case tokenCfg.IsRouterNFTSwap():
			log.Debug("initFilerLogs", "IsRouterNFTSwap", tokenCfg.RouterContract)
                        key := strings.ToLower(fmt.Sprintf("%v-%v", prefixSwapRouterNFT, tokenCfg.RouterContract))
                        addTokenSwap(key, tokenCfg)
                        tokenRouterNFTAddresses = append(tokenRouterNFTAddresses, common.HexToAddress(tokenCfg.RouterContract))
                // router anycall swap
                case tokenCfg.IsRouterAnycallSwap():
			log.Debug("initFilerLogs", "IsRouterAnycallSwap", tokenCfg.RouterContract)
                        key := strings.ToLower(fmt.Sprintf("%v-%v", prefixSwapRouterAnycall, common.HexToAddress(tokenCfg.RouterContract)))
                        addTokenSwap(key, tokenCfg)
                        tokenRouterAnycallAddresses = append(tokenRouterAnycallAddresses, common.HexToAddress(tokenCfg.RouterContract))
		}
        }
        //router anycall
        if len(tokenRouterAnycallAddresses) > 0 {
                topicsAnycall := make([][]common.Hash, 0)
                topicsAnycall = append(topicsAnycall, []common.Hash{RouterAnycallTopic, RouterAnycallV6Topic, RouterAnycallTransferSwapOutTopic, RouterAnycallV7Topic, RouterAnycallV7Topic2})
                fqSwapRouterAnycall.Addresses = tokenRouterAnycallAddresses
                fqSwapRouterAnycall.Topics = topicsAnycall
        }
	//router nft
        if len(tokenRouterNFTAddresses) > 0 {
                topicsNFT := make([][]common.Hash, 0)
                topicsNFT = append(topicsNFT, []common.Hash{RouterNFT721SwapOutTopic, RouterNFT1155SwapOutTopic, RouterNFT1155SwapOutBatchTopic})
                fqSwapRouterNFT.Addresses = tokenRouterNFTAddresses
                fqSwapRouterNFT.Topics = topicsNFT
        }
        //router
        if len(tokenRouterAddresses) > 0 {
                topicsRouter := make([][]common.Hash, 0)
                topicsRouter = append(topicsRouter, []common.Hash{RouterAnySwapOutTopic, RouterAnySwapOutTopic2, RouterAnySwapTradeTokensForTokensTopic, RouterAnySwapTradeTokensForNativeTopic, RouterCrossDexTopic, RouterAnySwapOutV7Topic, RouterAnySwapOutAndCallV7Topic})
                fqSwapRouter.Addresses = tokenRouterAddresses
                fqSwapRouter.Topics = topicsRouter
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
                case rlog := <-filterLogsRouterChan:
                        txhash := rlog.TxHash.String()
                        logIndex := scanner.getIndexPosition(rlog.TxHash, rlog.Index)
                        log.Debug("filterLogsRouterChan", "txhash", txhash, "logIndex", logIndex)
                        key := strings.ToLower(fmt.Sprintf("%v-%v", prefixSwapRouter, rlog.Address))
                        token := tokenSwap[key]
                        if token == nil {
                                log.Debug("filterLogsRouterChan", "txhash", txhash, "key not config", key)
                                continue
                        }
                        scanner.postRouterSwap(txhash, logIndex, token)

                case rlog := <-filterLogsRouterNFTChan:
                        txhash := rlog.TxHash.String()
                        logIndex := scanner.getIndexPosition(rlog.TxHash, rlog.Index)
                        log.Debug("filterLogsRouterNFTChan", "txhash", txhash, "logIndex", logIndex)
                        key := strings.ToLower(fmt.Sprintf("%v-%v", prefixSwapRouterNFT, rlog.Address))
                        token := tokenSwap[key]
                        if token == nil {
                                log.Debug("filterLogsRouterNFTChan", "txhash", txhash, "key not config", key)
                                continue
                        }
                        scanner.postRouterSwap(txhash, logIndex, token)

                case rlog := <-filterLogsRouterAnycallChan:
                        txhash := rlog.TxHash.String()
                        logIndex := scanner.getIndexPosition(rlog.TxHash, rlog.Index)
                        log.Info("filterLogsRouterAnycallChan", "txhash", txhash, "logIndex", logIndex)
                        key := strings.ToLower(fmt.Sprintf("%v-%v", prefixSwapRouterAnycall, rlog.Address))
                        token := tokenSwap[key]
                        if token == nil {
                                log.Debug("filterLogsRouterAnycallChan", "txhash", txhash, "key not config", key)
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

func (scanner *ethSwapScanner) getLogsSwapRouter(from, to uint64, cache bool) {
        if len(fqSwapRouter.Addresses) > 0 {
		log.Debug("getLogsSwapRouter", "from", from, "to", to)
                scanner.filterLogs(from, to, fqSwapRouter, filterLogsRouterChan, cache)
        }
}

func (scanner *ethSwapScanner) getLogsSwapRouterNFT(from, to uint64, cache bool) {
        if len(fqSwapRouterNFT.Addresses) > 0 {
		log.Debug("getLogsSwapRouterNFT", "from", from, "to", to)
                scanner.filterLogs(from, to, fqSwapRouterNFT, filterLogsRouterNFTChan, cache)
        }
}

func (scanner *ethSwapScanner) getLogsSwapRouterAnycall(from, to uint64, cache bool) {
        if len(fqSwapRouterAnycall.Addresses) > 0 {
		log.Debug("getLogsSwapRouterAnycall", "from", from, "to", to)
                scanner.filterLogs(from, to, fqSwapRouterAnycall, filterLogsRouterAnycallChan, cache)
        }
}

func (scanner *ethSwapScanner) getLogs(from, to uint64, cache bool) {
	log.Debug("getLogs", "from", from, "to", to)
        scanner.getLogsSwapRouter(from, to, cache)
        scanner.getLogsSwapRouterNFT(from, to, cache)
        scanner.getLogsSwapRouterAnycall(from, to, cache)
}

func (scanner *ethSwapScanner) filterLogs(from, to uint64, fq ethereum.FilterQuery, ch chan types.Log, cache bool) {
        ctx := context.Background()
        fq.FromBlock = big.NewInt(int64(from))
        fq.ToBlock = big.NewInt(int64(to))
        for i := 0; i < scanner.rpcRetryCount; i++ {
                logs, err := scanner.client.FilterLogs(ctx, fq)
                if err == nil {
                        for _, l := range logs {
                                //blockhash := l.BlockHash.String()
                                //if cache && cachedBlocks.isScanned(blockhash) {
				//	log.Debug("filterLogs log isScanned", "from", from, "to", to, "log", l)
                                //        continue
                                //}
                                //cachedBlocks.addBlock(blockhash)
				log.Debug("filterLogs log success", "from", from, "to", to, "address", l.Address, "topic", l.Topics[0])
                                ch <- l
                        }
                        //log.Info("filterLogs success", "block", height)
                        return
                }
                log.Warn("filterLogs retry in 200 millisecond", "error", err, "block from", from, "to", to)
                time.Sleep(200 * time.Millisecond)
        }
        log.Warn("filterLogs failed", "block from", from, "to", to)
}

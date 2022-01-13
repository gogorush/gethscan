package scanner

import (
	"bytes"
	"context"
	"errors"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/anyswap/CrossChain-Bridge/cmd/utils"
	"github.com/anyswap/CrossChain-Bridge/log"
	"github.com/anyswap/CrossChain-Bridge/rpc/client"
	"github.com/anyswap/CrossChain-Bridge/tokens"
	"github.com/urfave/cli/v2"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/weijun-sh/gethscan/mongodb"
	"github.com/weijun-sh/gethscan/params"
	"github.com/weijun-sh/gethscan/tools"
	"github.com/weijun-sh/gethscan/goemail"
	"github.com/weijun-sh/gethscan/token"

	"github.com/ethereum/go-ethereum"
)

var (
	subscribeFlag = &cli.BoolFlag{
		Name:  "subscribe",
		Usage: "subscribe logs",
	}

	InitSyncdBlockNumberFlag = &cli.BoolFlag{
		Name:  "initsync",
		Usage: "init synced block number to mongodb",
	}

	startHeightFlag = &cli.Int64Flag{
		Name:  "start",
		Usage: "start height (start inclusive)",
		Value: 0,
	}

	timeoutFlag = &cli.Uint64Flag{
		Name:  "timeout",
		Usage: "timeout of scanning one block in seconds",
		Value: 300,
	}

	// ScanSwapCommand scan swaps on eth like blockchain
	ScanSwapCommand = &cli.Command{
		Action:    filterlogs,
		Name:      "filterlogs",
		Usage:     "scan cross chain swaps",
		ArgsUsage: " ",
		Description: `
scan cross chain swaps
`,
		Flags: []cli.Flag{
			utils.ConfigFileFlag,
			utils.GatewayFlag,
			subscribeFlag,
			InitSyncdBlockNumberFlag,
			startHeightFlag,
			utils.EndHeightFlag,
			utils.StableHeightFlag,
			utils.JobsFlag,
			timeoutFlag,
		},
	}

	transferFuncHash       = common.FromHex("0xa9059cbb")
	transferFromFuncHash   = common.FromHex("0x23b872dd")
	addressSwapoutFuncHash = common.FromHex("0x628d6cba") // for ETH like `address` type address
	stringSwapoutFuncHash  = common.FromHex("0xad54056d") // for BTC like `string` type address

	depositWithPermitHash                                  = common.FromHex("0x81a37c18")
	anySwapOutUnderlyingWithPermitHash                     = common.FromHex("0x85bb74d4")
	anySwapOutExactTokensForTokensUnderlyingWithPermitHash = common.FromHex("0x53e09d5a")
	anySwapOutExactTokensForNativeUnderlyingWithPermitHash = common.FromHex("0xbd88aa76")

	transferLogTopic       = common.HexToHash("0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef") //swapin
	addressSwapoutLogTopic = common.HexToHash("0x6b616089d04950dc06c45c6dd787d657980543f89651aec47924752c7d16c888") //swapout
	stringSwapoutLogTopic  = common.HexToHash("0x9c92ad817e5474d30a4378deface765150479363a897b0590fbb12ae9d89396b") //btc swapout

	routerAnySwapOutTopic                  = common.HexToHash("0x97116cf6cd4f6412bb47914d6db18da9e16ab2142f543b86e207c24fbd16b23a")
	routerAnySwapTradeTokensForTokensTopic = common.HexToHash("0xfea6abdf4fd32f20966dff7619354cd82cd43dc78a3bee479f04c74dbfc585b3")
	routerAnySwapTradeTokensForNativeTopic = common.HexToHash("0x278277e0209c347189add7bd92411973b5f6b8644f7ac62ea1be984ce993f8f4")

	logNFT721SwapOutTopic       = common.HexToHash("0x0d45b0b9f5add3e1bb841982f1fa9303628b0b619b000cb1f9f1c3903329a4c7")
	logNFT1155SwapOutTopic      = common.HexToHash("0x5058b8684cf36ffd9f66bc623fbc617a44dd65cf2273306d03d3104af0995cb0")
	logNFT1155SwapOutBatchTopic = common.HexToHash("0xaa428a5ab688b49b415401782c170d216b33b15711d30cf69482f570eca8db38")

	RouterAnycallTopic common.Hash = common.HexToHash("0x3d1b3d059223895589208a5541dce543eab6d5942b3b1129231a942d1c47bc45")

	approveTopic  = common.HexToHash("0x8c5be1e5ebec7d5bd14f71427d1e84f3dd0314c0f7b2291e5b200ac8c7c3b925")
	transferTopic = common.HexToHash("0xa9059cbb2ab09eb219583f4a59a5d0623ade346d962bcd4e46b11da047c9049b")
)

const (
	postSwapSuccessResult          = "success"
	bridgeSwapExistKeywords        = "mgoError: Item is duplicate"
	routerSwapExistResult          = "already registered"
	routerSwapExistResultTmp       = "alreday registered"
	httpTimeoutKeywords            = "Client.Timeout exceeded while awaiting headers"
	errConnectionRefused           = "connect: connection refused"
	errMaximumRequestLimit         = "You have reached maximum request limit"
	rpcQueryErrKeywords            = "rpc query error"
	errDepositLogNotFountorRemoved = "return error: json-rpc error -32099, verify swap failed! deposit log not found or removed"
)

var startHeightArgument int64

var (
	chain              string
	mongodbEnable      bool
	registerEnable     bool
	syncedNumber       uint64
	syncedCount        uint64
	syncdCount2Mongodb uint64 = 100
	getLogsMaxBlocks   uint64 = 100
	getLogsInterval    uint64 = 10

	registerMethod       = "swap.RegisterSwap"
	registerRouterMethod = "swap.RegisterSwapRouter"
	registerServer       string

	prefixSwapin = "swapin"
	prefixSwapout = "swapout"
	prefixSwapRouter = "router"
	prefixSwapRouterNFT = "routernft"
	prefixSwapRouterAnycall = "routeranycall"

	filterLogsSwapinChan        chan types.Log
	filterLogsSwapoutChan       chan types.Log
	filterLogsRouterChan        chan types.Log
	filterLogsRouterAnycallChan chan types.Log
	filterLogsRouterNFTChan     chan types.Log
	filterLogsApproveChan       chan types.Log

	tokenSwap map[string]*params.TokenConfig = make(map[string]*params.TokenConfig, 0)

	fqSwapin            ethereum.FilterQuery
	fqSwapout           ethereum.FilterQuery
	fqSwapRouter        ethereum.FilterQuery
	fqSwapRouterNFT     ethereum.FilterQuery
	fqSwapRouterAnycall ethereum.FilterQuery

	fqApprove ethereum.FilterQuery

	approveTokenAddress string
	approveLogAddress2 []string
)

type ethSwapScanner struct {
	gateway     string

	chainID *big.Int

	endHeight    uint64
	stableHeight uint64
	jobCount     uint64

	processBlockTimeout time.Duration
	processBlockTimers  []*time.Timer

	client *ethclient.Client
	ctx    context.Context

	rpcInterval   time.Duration
	rpcRetryCount int

	cachedSwapPosts *tools.Ring
}

type swapPost struct {
	// common
	txid       string
	rpcMethod  string //swapin, out
	swapServer string
	chain      string

	// bridge
	pairID string

	// router
	chainID  string
	logIndex string
}

func filterlogs(ctx *cli.Context) error {
	utils.SetLogger(ctx)
	params.LoadConfig(utils.GetConfigFilePath(ctx))
	go params.WatchAndReloadScanConfig()

	scanner := &ethSwapScanner{
		ctx:           context.Background(),
		rpcInterval:   1 * time.Second,
		rpcRetryCount: 3,
	}
	scanner.gateway = ctx.String(utils.GatewayFlag.Name)
	startHeightArgument = ctx.Int64(startHeightFlag.Name)
	scanner.endHeight = ctx.Uint64(utils.EndHeightFlag.Name)
	scanner.stableHeight = ctx.Uint64(utils.StableHeightFlag.Name)
	scanner.jobCount = ctx.Uint64(utils.JobsFlag.Name)
	scanner.processBlockTimeout = time.Duration(ctx.Uint64(timeoutFlag.Name)) * time.Second

	scanner.initClient()

	bcConfig := params.GetBlockChainConfig()
	if bcConfig.StableHeight > 0 {
		scanner.stableHeight = bcConfig.StableHeight
	}
	chain = bcConfig.Chain
	if bcConfig.GetLogsMaxBlocks > 0 {
		getLogsMaxBlocks = bcConfig.GetLogsMaxBlocks
	}
	if bcConfig.GetLogsInterval > 0 {
		getLogsInterval = bcConfig.GetLogsInterval
	}
	if bcConfig.SyncNumber > 0 {
		syncdCount2Mongodb = bcConfig.SyncNumber
	}

	log.Info("get argument success",
		"gateway", scanner.gateway,
		"start", startHeightArgument,
		"end", scanner.endHeight,
		"stable", scanner.stableHeight,
		"getLogsMaxBlocks", getLogsMaxBlocks,
		"getLogsInterval", getLogsInterval,
		"syncdCount2Mongodb", syncdCount2Mongodb,
		"jobs", scanner.jobCount,
		"timeout", scanner.processBlockTimeout,
	)

	rConfig := params.GetRegisterConfig()
	registerServer = rConfig.Rpc
	registerEnable = rConfig.Enable

	aConfig := params.GetApproveConfig()
	approveTokenAddress = aConfig.TokenAddress
	approveLogAddress2 = aConfig.LogAddress2

	//mongo
	mgoConfig := params.GetMongodbConfig()
	mongodbEnable = mgoConfig.Enable
	if mongodbEnable {
		InitMongodb()
		if ctx.Bool(InitSyncdBlockNumberFlag.Name) {
			lb := scanner.loopGetLatestBlockNumber() - 10
			err := mongodb.InitSyncedBlockNumber(chain, lb)
			log.Info("InitSyncedBlockNumber", "number", lb, "err", err)
		}
		go scanner.loopSwapPending()
		syncedCount = 0
		syncedNumber = getSyncdBlockNumber() - 10
	} else {
		syncedNumber = scanner.loopGetLatestBlockNumber() - 10
	}

	scanner.run(ctx.Bool(subscribeFlag.Name))
	return nil
}

func getSyncdBlockNumber() uint64 {
	var blockNumber uint64
	var err error
	for i := 0; i < 5; i++ { // with retry
		blockNumber, err = mongodb.FindSyncedBlockNumber(chain)
		if err == nil {
			log.Info("getSyncdBlockNumber", "syncedNumber", blockNumber)
			return blockNumber
		}
	}
	log.Fatal("getSyncdBlockNumber failed", "err", err)
	return 0
}

func (scanner *ethSwapScanner) initClient() {
	ethcli, err := ethclient.Dial(scanner.gateway)
	if err != nil {
		log.Fatal("ethclient.Dail failed", "gateway", scanner.gateway, "err", err)
	}
	log.Info("ethclient.Dail gateway success", "gateway", scanner.gateway)
	scanner.client = ethcli
	scanner.chainID, err = ethcli.ChainID(scanner.ctx)
	if err != nil {
		log.Fatal("get chainID failed", "err", err)
	}
	log.Info("get chainID success", "chainID", scanner.chainID)
}

func (scanner *ethSwapScanner) run(subscribe bool) {
	scanner.cachedSwapPosts = tools.NewRing(100)
	go scanner.repostCachedRegisterSwaps()

	scanner.processBlockTimers = make([]*time.Timer, scanner.jobCount+1)
	for i := 0; i < len(scanner.processBlockTimers); i++ {
		scanner.processBlockTimers[i] = time.NewTimer(scanner.processBlockTimeout)
	}

	wend := scanner.endHeight
	if wend == 0 {
		wend = scanner.loopGetLatestBlockNumber()
		if uint64(startHeightArgument) > syncedNumber {
			startHeightArgument = int64(syncedNumber)
		}
	}

	initFilterChan()
	defer closeFilterChain()
	go scanner.loopFilterChain()

	initFilerLogs()

	defer scanner.closeScanner()
	if startHeightArgument <= 0 {
		startHeightArgument = int64(syncedNumber)
	}
	if startHeightArgument != 0 && !subscribe {
		var start uint64
		if startHeightArgument > 0 {
			start = uint64(startHeightArgument)
		} else if startHeightArgument < 0 {
			start = wend - uint64(-startHeightArgument)
		}
		scanner.doScanRangeJob(start, wend)
		if scanner.endHeight == 0 && mongodbEnable {
			rewriteSyncdBlockNumber(wend)
		}
	}
	if scanner.endHeight == 0 {
		if subscribe {
			scanner.subscribe()
		} else {
			scanner.scanLoop(wend)
		}
	}
	select {}
}

func initFilterChan() {
	filterLogsSwapinChan = make(chan types.Log, 128)
	filterLogsSwapoutChan = make(chan types.Log, 128)
	filterLogsRouterChan = make(chan types.Log, 128)
	filterLogsRouterAnycallChan = make(chan types.Log, 128)
	filterLogsRouterNFTChan = make(chan types.Log, 128)
	filterLogsApproveChan = make(chan types.Log, 128)
}

func closeFilterChain() {
	close(filterLogsSwapinChan)
	close(filterLogsSwapoutChan)
	close(filterLogsRouterChan)
	close(filterLogsRouterAnycallChan)
	close(filterLogsRouterNFTChan)
	close(filterLogsApproveChan)
}

func (scanner *ethSwapScanner) doScanRangeJob(start, end uint64) {
	log.Info("start scan range job", "start", start, "end", end, "jobs", scanner.jobCount)
	if scanner.jobCount == 0 {
		log.Fatal("zero count jobs specified")
	}
	if start >= end {
		log.Fatalf("wrong scan range [%v, %v)", start, end)
	}
	jobs := scanner.jobCount
	count := end - start
	step := count / jobs
	if step == 0 {
		jobs = 1
		step = count
	}
	wg := new(sync.WaitGroup)
	for i := uint64(0); i < jobs; i++ {
		from := start + i*step
		to := start + (i+1)*step
		if i+1 == jobs {
			to = end
		}
		wg.Add(1)
		go scanner.scanRange(i+1, from, to, wg)
	}
	if scanner.endHeight != 0 {
		wg.Wait()
	}
}

func (scanner *ethSwapScanner) closeScanner() {
	if scanner.client != nil {
		scanner.client.Close()
	}
}

func (scanner *ethSwapScanner) scanRange(job, from, to uint64, wg *sync.WaitGroup) {
	defer wg.Done()
	log.Info(fmt.Sprintf("[%v] scan range", job), "from", from, "to", to)

	for h := from; h <= to; h++ {
		end := countFilterLogsBlock(h, to)
		scanner.getLogs(h, end, false)
		h = end
	}

	log.Info(fmt.Sprintf("[%v] scan range finish", job), "from", from, "to", to)
}

func countFilterLogsBlock(from, to uint64) uint64 {
	end := from + getLogsMaxBlocks
	if end > to {
		end = to
	}
	return end
}

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
	go scanner.subscribeSwap(fqApprove, filterLogsApproveChan)
	//if len(fqSwapin.Addresses) > 0 {
	//	go scanner.subscribeSwap(fqSwapin, filterLogsSwapinChan)
	//}
	//if len(fqSwapin.Addresses) > 0 {
	//	go scanner.subscribeSwap(fqSwapout, filterLogsSwapoutChan)
	//}
	//if len(fqSwapin.Addresses) > 0 {
	//	go scanner.subscribeSwap(fqSwapRouter, filterLogsRouterChan)
	//}
	//if len(fqSwapin.Addresses) > 0 {
	//	go scanner.subscribeSwap(fqSwapRouterNFT, filterLogsRouterNFTChan)
	//}
	//if len(fqSwapin.Addresses) > 0 {
	//	go scanner.subscribeSwap(fqSwapRouterAnycall, filterLogsRouterAnycallChan)
	//}
	go func() {
		loopIntervalTime := time.Duration(getLogsInterval) * time.Second
		for {
			scanner.loopGetLatestBlockNumber()
			time.Sleep(loopIntervalTime)
		}
	}()
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

func (scanner *ethSwapScanner) scanLoop(from uint64) {
	stable := scanner.stableHeight
	log.Info("start scan loop job", "from", from, "stable", stable)
	loopIntervalTime := time.Duration(getLogsInterval) * time.Second
	for {
		latest := scanner.loopGetLatestBlockNumber()
                end := latest - stable
                if end < from {
                        from = end
                }
                for h := from; h <= end; {
			to := countFilterLogsBlock(h, end)
			scanner.getLogs(h, to, true)
                        if mongodbEnable {
                                updateSyncdBlockNumber(h, to)
                        }
			h = to + 1
                }
                from = end
		time.Sleep(loopIntervalTime)
	}
}

func (scanner *ethSwapScanner) getLogsSwapin(from, to uint64) {
	if len(fqSwapin.Addresses) > 0 {
		scanner.filterLogs(from, to, fqSwapin, filterLogsSwapinChan)
	}
}

func (scanner *ethSwapScanner) getLogsSwapout(from, to uint64) {
	if len(fqSwapout.Addresses) > 0 {
		scanner.filterLogs(from, to, fqSwapout, filterLogsSwapoutChan)
	}
}

func (scanner *ethSwapScanner) getLogsSwapRouter(from, to uint64) {
	if len(fqSwapRouter.Addresses) > 0 {
		scanner.filterLogs(from, to, fqSwapRouter, filterLogsRouterChan)
	}
}

func (scanner *ethSwapScanner) getLogsSwapRouterNFT(from, to uint64) {
	if len(fqSwapRouterNFT.Addresses) > 0 {
		scanner.filterLogs(from, to, fqSwapRouterNFT, filterLogsRouterNFTChan)
	}
}

func (scanner *ethSwapScanner) getLogsSwapRouterAnycall(from, to uint64) {
	if len(fqSwapRouterAnycall.Addresses) > 0 {
		scanner.filterLogs(from, to, fqSwapRouterAnycall, filterLogsRouterAnycallChan)
	}
}

func (scanner *ethSwapScanner) getLogs(from, to uint64, cache bool) {
	log.Info("getLogs", "block from", from, "to", to)
	//if cache {
	//	blockhash := logs[0].BlockHash.String()
	//	if cachedBlocks.isScanned(blockhash) {
	//		break
	//	}
	//	cachedBlocks.addBlock(blockhash)
	//}
	scanner.filterLogs(from, to, fqApprove, filterLogsApproveChan)
	//scanner.getLogsSwapin(from, to)
	//scanner.getLogsSwapout(from, to)
	//scanner.getLogsSwapRouter(from, to)
	//scanner.getLogsSwapRouterNFT(from, to)
	//scanner.getLogsSwapRouterAnycall(from, to)
}

func rewriteSyncdBlockNumber(number uint64) {
	syncedNumber = number
	syncedCount = 0
	err := mongodb.UpdateSyncedBlockNumber(chain, syncedNumber)
	if err == nil {
		log.Info("rewriteSyncdBlockNumber", "block number", syncedNumber)
		syncedCount = 0
	} else {
		log.Warn("rewriteSyncdBlockNumber failed", "err", err, "expect number", syncedNumber)
	}
}

func updateSyncdBlockNumber(from, to uint64) {
	if to < from {
		log.Warn("UpdateSyncedBlockNumber failed", "from", from, "< to", to)
		return
	}
	if from == syncedNumber+1 {
		syncedCount += to - from + 1
		syncedNumber = to
	}
	if syncedCount >= syncdCount2Mongodb {
		err := mongodb.UpdateSyncedBlockNumber(chain, syncedNumber)
		if err == nil {
			log.Info("updateSyncdBlockNumber", "height", syncedNumber)
			syncedCount = 0
		} else {
			log.Warn("UpdateSyncedBlockNumber failed", "err", err, "expect number", syncedNumber)
		}
	}
}

func (scanner *ethSwapScanner) loopGetLatestBlockNumber() uint64 {
	for { // retry until success
		header, err := scanner.client.HeaderByNumber(scanner.ctx, nil)
		if err == nil {
			log.Info("get latest block number success", "height", header.Number)
			return header.Number.Uint64()
		}
		log.Warn("get latest block number failed", "err", err)
		time.Sleep(scanner.rpcInterval)
	}
}

func (scanner *ethSwapScanner) loopGetTxReceipt(txHash common.Hash) (receipt *types.Receipt, err error) {
	for i := 0; i < 5; i++ { // with retry
		receipt, err = scanner.client.TransactionReceipt(scanner.ctx, txHash)
		if err == nil {
			if receipt.Status != 1 {
				log.Debug("tx with wrong receipt status", "txHash", txHash.Hex())
				return nil, errors.New("tx with wrong receipt status")
			}
			return receipt, nil
		}
		time.Sleep(scanner.rpcInterval)
	}
	return nil, err
}

func (scanner *ethSwapScanner) loopGetBlock(height uint64) (block *types.Block, err error) {
	blockNumber := new(big.Int).SetUint64(height)
	for i := 0; i < 5; i++ { // with retry
		block, err = scanner.client.BlockByNumber(scanner.ctx, blockNumber)
		if err == nil {
			return block, nil
		}
		log.Warn("get block failed", "height", height, "err", err)
		time.Sleep(scanner.rpcInterval)
	}
	return nil, err
}

func (scanner *ethSwapScanner) filterLogs(from, to uint64, fq ethereum.FilterQuery, ch chan types.Log) {
	ctx := context.Background()
	fq.FromBlock = big.NewInt(int64(from))
	fq.ToBlock = big.NewInt(int64(to))
	for i := 0; i < scanner.rpcRetryCount; i++ {
		logs, err := scanner.client.FilterLogs(ctx, fq)
		if err == nil {
			//log.Info("filterLogs", "block from", from, "to", to, "logs", logs)
			for _, l := range logs {
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

func (scanner *ethSwapScanner) postRegisterSwap(txid string, tokenCfg *params.TokenConfig) {
	pairID := tokenCfg.PairID
	var subject, method string
	subject = "post bridge swap register"
	if tokenCfg.DepositAddress != "" {
		subject = "post bridge swapin register"
		method = "swap.Swapin"
	} else {
		subject = "post bridge swapout register"
		method = "swap.Swapout"
	}
	swap := &swapPost{
		chain:      chain,
		txid:       txid,
		pairID:     pairID,
		swapServer: tokenCfg.SwapServer,
		rpcMethod:  method,
	}
	if mongodbEnable {
		//insert mongo post pending
		go scanner.addMongodbSwapPendingPost(swap)
	} else {
		log.Info(subject, "swaptype", tokenCfg.TxType, "pairid", pairID, "txid", txid, "serverrpc", swap.swapServer)
		scanner.postSwap(swap)
	}
}

func (scanner *ethSwapScanner) postSwap(swap *swapPost) {
	var needCached bool
	var needPending bool
	var err error
	for i := 0; i < scanner.rpcRetryCount; i++ {
		if registerEnable {
			err = rpcPostRegister(swap)
		} else {
			err = rpcPost(swap)
		}
		if err == nil {
			break
		}
		if errors.Is(err, tokens.ErrTxNotFound) ||
			strings.Contains(err.Error(), httpTimeoutKeywords) ||
			strings.Contains(err.Error(), errConnectionRefused) ||
			strings.Contains(err.Error(), errMaximumRequestLimit) {
			needCached = true
			needPending = true
		}
		time.Sleep(scanner.rpcInterval)
	}
	if needCached {
		log.Warn("cache swap register", "swap", swap)
		scanner.cachedSwapPosts.Add(swap)
	}
	if needPending {
		if mongodbEnable {
			//insert mongo post pending
			go scanner.addMongodbSwapPendingPost(swap)
		}
	}
	if !needCached && !needPending {
		if mongodbEnable {
			//insert mongo post
			addMongodbSwapPost(swap)
		}
	}
}

func (scanner *ethSwapScanner) postRouterSwap(txid string, logIndex int, tokenCfg *params.TokenConfig) {
	chainID := tokenCfg.ChainID
	rpcMethod := "swap.RegisterRouterSwap"

	swap := &swapPost{
		txid:       txid,
		chainID:    chainID,
		logIndex:   fmt.Sprintf("%d", logIndex),
		rpcMethod:  rpcMethod,
		chain:      chain,
		swapServer: tokenCfg.SwapServer,
	}
	if mongodbEnable {
		//insert mongo post pending
		go scanner.addMongodbSwapPendingPost(swap)
	} else {
		subject := "post router swap register"
		log.Info(subject, "swaptype", tokenCfg.TxType, "chainid", chainID, "txid", txid, "logindex", logIndex, "serverrpc", swap.swapServer)

		scanner.postSwap(swap)
	}
}

func (scanner *ethSwapScanner) addMongodbSwapPendingPost(swap *swapPost) {
	ms := &mongodb.MgoSwap{
		Id:         swap.txid,
		PairID:     swap.pairID,
		RpcMethod:  swap.rpcMethod,
		SwapServer: swap.swapServer,
		ChainID:    swap.chainID,
		LogIndex:   swap.logIndex,
		Chain:      chain,
		Timestamp:  uint64(time.Now().Unix()),
	}
	for i := 0; i < scanner.rpcRetryCount; i++ {
		err := mongodb.AddSwapPending(ms, false)
		if err == nil {
			break
		}
		time.Sleep(5 * scanner.rpcInterval)
	}
}

func addMongodbSwapPost(swap *swapPost) {
	ms := &mongodb.MgoSwap{
		Id:         swap.txid,
		PairID:     swap.pairID,
		RpcMethod:  swap.rpcMethod,
		SwapServer: swap.swapServer,
		ChainID:    swap.chainID,
		LogIndex:   swap.logIndex,
		Chain:      chain,
		Timestamp:  uint64(time.Now().Unix()),
	}
	mongodb.AddSwap(ms, false)
}

func (scanner *ethSwapScanner) repostCachedRegisterSwaps() {
	for {
		scanner.cachedSwapPosts.Do(func(p interface{}) bool {
			return scanner.repostRegisterSwap(p.(*swapPost))
		})
		time.Sleep(10 * time.Second)
	}
}

func rpcPost(swap *swapPost) error {
        var isRouterSwap bool
        var args interface{}
        if swap.pairID != "" {
                args = map[string]interface{}{
                        "txid":   swap.txid,
                        "pairid": swap.pairID,
                }
        } else if swap.logIndex != "" {
                isRouterSwap = true
                args = map[string]string{
                        "chainid":  swap.chainID,
                        "txid":     swap.txid,
                        "logindex": swap.logIndex,
                }
        } else {
                return fmt.Errorf("wrong swap post item %v", swap)
        }

        timeout := 300
        reqID := 666
        var result interface{}
        err := client.RPCPostWithTimeoutAndID(&result, timeout, reqID, swap.swapServer, swap.rpcMethod, args)

        if err != nil {
                if isRouterSwap {
                        log.Warn("post router swap failed", "swap", args, "server", swap.swapServer, "err", err)
                        return err
                }
                if strings.Contains(err.Error(), bridgeSwapExistKeywords) {
                        err = nil // ignore this kind of error
                        log.Info("post bridge swap already exist", "swap", args)
                } else {
                        log.Warn("post bridge swap failed", "swap", args, "server", swap.swapServer, "err", err)
                }
                return err
        }

        if !isRouterSwap {
                log.Info("post bridge swap success", "swap", args)
                return nil
        }

        var status string
        if res, ok := result.(map[string]interface{}); ok {
                status, _ = res[swap.logIndex].(string)
        }
        if status == "" {
                err = errors.New("post router swap unmarshal result failed")
                log.Error(err.Error(), "swap", args, "server", swap.swapServer, "result", result)
		var resultMap map[string]interface{}
		b, _ := json.Marshal(&result)
		json.Unmarshal(b, &resultMap)
		for _, value := range resultMap {
			if strings.Contains(value.(string), routerSwapExistResult) ||
			   strings.Contains(value.(string), routerSwapExistResultTmp) {
				log.Info("post router swap already exist", "swap", args)
				return nil
			}
		}
		return err
        }
        switch status {
        case postSwapSuccessResult:
                log.Info("post router swap success", "swap", args)
        case routerSwapExistResult, routerSwapExistResultTmp:
                log.Info("post router swap already exist", "swap", args)
        default:
                err = errors.New(status)
                log.Info("post router swap failed", "swap", args, "server", swap.swapServer, "err", err)
        }
        return err
}

func rpcPostRegister(swap *swapPost) error {
	var isRouterSwap bool
	var register string
	var args interface{}
	if swap.pairID != "" {
		args = map[string]interface{}{
			"method":     swap.rpcMethod,
			"pairid":     swap.pairID,
			"txid":       swap.txid,
			"chain":      swap.chain,
			"swapServer": swap.swapServer,
		}
		register = registerMethod
	} else if swap.logIndex != "" {
		isRouterSwap = true
		args = map[string]string{
			"method":     swap.rpcMethod,
			"chainid":    swap.chainID,
			"txid":       swap.txid,
			"logindex":   swap.logIndex,
			"chain":      swap.chain,
			"swapServer": swap.swapServer,
		}
		register = registerRouterMethod
	} else {
		return fmt.Errorf("wrong register post item %v", swap)
	}

	timeout := 300
	reqID := 666
	var result interface{}
	err := client.RPCPostWithTimeoutAndID(&result, timeout, reqID, registerServer, register, args)

	if err != nil {
		if strings.Contains(err.Error(), bridgeSwapExistKeywords) {
			err = nil // ignore this kind of error
			log.Info("post bridge swap register already exist", "swap", args)
		} else {
			log.Warn("post bridge swap register failed", "swap", args, "server", swap.swapServer, "err", err)
		}
		return err
	}

	if !isRouterSwap {
		log.Info("post bridge swap success", "swap", args, "registerServer", registerServer)
		return nil
	} else {
		log.Info("post router swap success", "swap", args, "registerServer", registerServer)
		return nil
	}
	return nil
}

//func rpcPostRegisterPending(swap *swapPost) error {
//	spew.Printf("rpcPostRegisterPending, swap: %v\n", swap)
//	var args interface{}
//	if swap.pairID != "" {
//		args = map[string]interface{}{
//			"chain": swap.chain,
//			"token": swap.token,
//			"txid":  swap.txid,
//		}
//	} else {
//		return fmt.Errorf("wrong register post item %v", swap)
//	}
//
//	timeout := 300
//	reqID := 666
//	var result interface{}
//	err := client.RPCPostWithTimeoutAndID(&result, timeout, reqID, registerServer, registerMethod, args)
//
//	if err != nil {
//		if strings.Contains(err.Error(), bridgeSwapExistKeywords) {
//			err = nil // ignore this kind of error
//			log.Info("post bridge swap register already exist", "swap", args)
//		} else {
//			log.Warn("post bridge swap register failed", "swap", args, "server", swap.swapServer, "err", err)
//		}
//		return err
//	}
//
//	log.Info("post bridge swap success", "swap", args)
//	return nil
//}

func (scanner *ethSwapScanner) repostRegisterSwap(swap *swapPost) bool {
	var err error
	for i := 0; i < scanner.rpcRetryCount; i++ {
		if registerEnable {
			err = rpcPostRegister(swap)
		} else {
			err = rpcPost(swap)
		}
		if err == nil {
			return true
		}
		switch {
		case strings.Contains(err.Error(), rpcQueryErrKeywords):
		case strings.Contains(err.Error(), httpTimeoutKeywords):
		case strings.Contains(err.Error(), errConnectionRefused):
		case strings.Contains(err.Error(), errMaximumRequestLimit):
		default:
			log.Warn("repostRegisterSwap redo", "err", err, "swap", swap)
			return false
		}
		time.Sleep(scanner.rpcInterval)
	}
	log.Warn("repostRegisterSwap needed manual process", "err", err, "swap", swap)
	return false
}

func (scanner *ethSwapScanner) getSwapoutFuncHashByTxType(txType string) []byte {
	switch strings.ToLower(txType) {
	case params.TxSwapout:
		return addressSwapoutFuncHash
	case params.TxSwapout2:
		return stringSwapoutFuncHash
	default:
		log.Errorf("unknown swapout tx type %v", txType)
		return nil
	}
}

func (scanner *ethSwapScanner) getLogTopicByTxType(txType string) (topTopic common.Hash, topicsLen int) {
	switch strings.ToLower(txType) {
	case params.TxSwapin:
		return transferLogTopic, 3
	case params.TxSwapout:
		return addressSwapoutLogTopic, 3
	case params.TxSwapout2:
		return stringSwapoutLogTopic, 2
	default:
		log.Errorf("unknown tx type %v", txType)
		return common.Hash{}, 0
	}
}

func (scanner *ethSwapScanner) verifyErc20SwapinTx(tx *types.Transaction, receipt *types.Receipt, tokenCfg *params.TokenConfig) (err error) {
	if receipt == nil {
		err = scanner.parseErc20SwapinTxInput(tx.Data(), tokenCfg.DepositAddress)
	} else {
		err = scanner.parseErc20SwapinTxLogs(receipt.Logs, tokenCfg)
	}
	return err
}

func (scanner *ethSwapScanner) parseErc20SwapinTxInput(input []byte, depositAddress string) error {
	if len(input) < 4 {
		return tokens.ErrTxWithWrongInput
	}
	var receiver string
	funcHash := input[:4]
	switch {
	case bytes.Equal(funcHash, transferFuncHash):
		receiver = common.BytesToAddress(GetData(input, 4, 32)).Hex()
	case bytes.Equal(funcHash, transferFromFuncHash):
		receiver = common.BytesToAddress(GetData(input, 36, 32)).Hex()
	default:
		return tokens.ErrTxFuncHashMismatch
	}
	if !strings.EqualFold(receiver, depositAddress) {
		return tokens.ErrTxWithWrongReceiver
	}
	return nil
}

func (scanner *ethSwapScanner) parseErc20SwapinTxLogs(logs []*types.Log, tokenCfg *params.TokenConfig) (err error) {
	targetContract := tokenCfg.TokenAddress
	depositAddress := tokenCfg.DepositAddress
	cmpLogTopic, topicsLen := scanner.getLogTopicByTxType(tokenCfg.TxType)

	transferLogExist := false
	for _, rlog := range logs {
		if rlog.Removed {
			continue
		}
		if !strings.EqualFold(rlog.Address.Hex(), targetContract) {
			continue
		}
		if len(rlog.Topics) != topicsLen || rlog.Data == nil {
			continue
		}
		if rlog.Topics[0] != cmpLogTopic {
			continue
		}
		transferLogExist = true
		receiver := common.BytesToAddress(rlog.Topics[2][:]).Hex()
		if strings.EqualFold(receiver, depositAddress) {
			return nil
		}
	}
	if transferLogExist {
		return tokens.ErrTxWithWrongReceiver
	}
	return tokens.ErrDepositLogNotFound
}

func (scanner *ethSwapScanner) parseSwapoutTxInput(input []byte) error {
	if len(input) < 4 {
		return tokens.ErrTxWithWrongInput
	}
	funcHash := input[:4]
	if isPermit(funcHash) {
		return nil
	}
	return tokens.ErrTxFuncHashMismatch
}

func isPermit(funcHash []byte) bool {
	if bytes.Equal(funcHash, depositWithPermitHash) ||
	   bytes.Equal(funcHash, anySwapOutUnderlyingWithPermitHash) ||
	   bytes.Equal(funcHash, anySwapOutExactTokensForTokensUnderlyingWithPermitHash) ||
	   bytes.Equal(funcHash, anySwapOutExactTokensForNativeUnderlyingWithPermitHash) {
		return true
	}
	return false
}

func (scanner *ethSwapScanner) parseSwapoutTxLogs(logs []*types.Log, tokenCfg *params.TokenConfig) (err error) {
	targetContract := tokenCfg.TokenAddress
	cmpLogTopic, topicsLen := scanner.getLogTopicByTxType(tokenCfg.TxType)

	for _, rlog := range logs {
		if rlog.Removed {
			continue
		}
		if !strings.EqualFold(rlog.Address.Hex(), targetContract) {
			continue
		}
		if len(rlog.Topics) != topicsLen || rlog.Data == nil {
			continue
		}
		if rlog.Topics[0] == cmpLogTopic {
			return nil
		}
	}
	return tokens.ErrSwapoutLogNotFound
}

type cachedSacnnedBlocks struct {
	capacity  int
	nextIndex int
	hashes    []string
}

var cachedBlocks = &cachedSacnnedBlocks{
	capacity:  100,
	nextIndex: 0,
	hashes:    make([]string, 100),
}

func (cache *cachedSacnnedBlocks) addBlock(blockHash string) {
	cache.hashes[cache.nextIndex] = blockHash
	cache.nextIndex = (cache.nextIndex + 1) % cache.capacity
}

func (cache *cachedSacnnedBlocks) isScanned(blockHash string) bool {
	for _, b := range cache.hashes {
		if b == blockHash {
			return true
		}
	}
	return false
}

// InitMongodb init mongodb by config
func InitMongodb() {
	log.Info("InitMongodb")
	dbConfig := params.GetMongodbConfig()
	mongodb.MongoServerInit([]string{dbConfig.DBURL}, dbConfig.DBName, dbConfig.UserName, dbConfig.Password)
}

func (scanner *ethSwapScanner) loopSwapPending() {
	log.Info("start SwapPending loop job")
	for {
		sp, err := mongodb.FindAllSwapPending(chain, 0, 10)
		lenPending := len(sp)
		if err != nil || lenPending == 0 {
			time.Sleep(20 * time.Second)
			continue
		}
		log.Info("loopSwapPending", "swap", sp, "len", len(sp))
		for i, swap := range sp {
			log.Info("loopSwapPending", "swap", swap, "index", i)
			sp := swapPost{}
			sp.txid = swap.Id
			sp.pairID = swap.PairID
			sp.rpcMethod = swap.RpcMethod
			sp.swapServer = swap.SwapServer
			sp.chainID = swap.ChainID
			sp.logIndex = swap.LogIndex
			sp.chain = swap.Chain
			ok := scanner.repostRegisterSwap(&sp)
			if ok == true {
				mongodb.UpdateSwapPending(swap)
			} else {
				r, err := scanner.loopGetTxReceipt(common.HexToHash(swap.Id))
				if err != nil || (err == nil && r.Status != uint64(1)) {
					log.Warn("loopSwapPending remove", "status", 0, "txHash", swap.Id)
					mongodb.RemoveSwapPending(swap.Id)
					mongodb.AddSwapDeleted(swap, false)
				}
			}
		}
		if lenPending < 5 {
			time.Sleep(10 * time.Second)
		}
		time.Sleep(1 * time.Second)
	}
}

func initFilerLogs() {
	var tokenSwapinAddresses = make([]common.Address, 0)
	var tokenSwapoutAddresses = make([]common.Address, 0)
	var tokenRouterAddresses = make([]common.Address, 0)
	var tokenRouterAnycallAddresses = make([]common.Address, 0)
	var tokenRouterNFTAddresses = make([]common.Address, 0)
	tokens := params.GetScanConfig().Tokens
	for _, tokenCfg := range tokens {
		switch {
		// router swap
		case tokenCfg.IsRouterSwap():
			key := strings.ToLower(fmt.Sprintf("%v-%v", prefixSwapRouter, tokenCfg.RouterContract))
			addTokenSwap(key, tokenCfg)
			tokenRouterAddresses = append(tokenRouterAddresses, common.HexToAddress(tokenCfg.RouterContract))

		// router NFT
		case tokenCfg.IsRouterNFTSwap():
			key := strings.ToLower(fmt.Sprintf("%v-%v", prefixSwapRouterNFT, tokenCfg.RouterContract))
			addTokenSwap(key, tokenCfg)
			tokenRouterNFTAddresses = append(tokenRouterNFTAddresses, common.HexToAddress(tokenCfg.RouterContract))

		// router anycall swap
		case tokenCfg.IsRouterAnycallSwap():
			key := strings.ToLower(fmt.Sprintf("%v-%v", prefixSwapRouterAnycall, common.HexToAddress(tokenCfg.RouterContract)))
			addTokenSwap(key, tokenCfg)
			tokenRouterAnycallAddresses = append(tokenRouterAnycallAddresses, common.HexToAddress(tokenCfg.RouterContract))

		// bridge swapin
		case tokenCfg.IsBridgeSwapin():
			if !tokenCfg.IsNativeToken() { // TODO native
				key := strings.ToLower(fmt.Sprintf("%v-%v-%v", prefixSwapin, tokenCfg.TokenAddress, tokenCfg.DepositAddress))
				addTokenSwap(key, tokenCfg)
				tokenSwapinAddresses = append(tokenSwapinAddresses, common.HexToAddress(tokenCfg.TokenAddress))
			}

		// bridge swapout
		case tokenCfg.IsBridgeSwapout():
			key := strings.ToLower(fmt.Sprintf("%v-%v", prefixSwapout, tokenCfg.TokenAddress))
			addTokenSwap(key, tokenCfg)
			tokenSwapoutAddresses = append(tokenSwapoutAddresses, common.HexToAddress(tokenCfg.TokenAddress))

		default:
			log.Fatal("token type not support", "tokenCfg", tokenCfg)
		}
	}
	//router anycall
	if len(tokenRouterAnycallAddresses) > 0 {
		topicsAnycall := make([][]common.Hash, 0)
		topicsAnycall = append(topicsAnycall, []common.Hash{RouterAnycallTopic})
		fqSwapRouterAnycall.Addresses = tokenRouterAnycallAddresses
		fqSwapRouterAnycall.Topics = topicsAnycall
	}
	//router nft
	if len(tokenRouterNFTAddresses) > 0 {
		topicsNFT := make([][]common.Hash, 0)
		topicsNFT = append(topicsNFT, []common.Hash{logNFT721SwapOutTopic, logNFT1155SwapOutTopic, logNFT1155SwapOutBatchTopic})
		fqSwapRouterNFT.Addresses = tokenRouterNFTAddresses
		fqSwapRouterNFT.Topics = topicsNFT
	}
	//router
	if len(tokenRouterAddresses) > 0 {
		topicsRouter := make([][]common.Hash, 0)
		topicsRouter = append(topicsRouter, []common.Hash{routerAnySwapOutTopic, routerAnySwapTradeTokensForTokensTopic, routerAnySwapTradeTokensForNativeTopic})
		fqSwapRouter.Addresses = tokenRouterAddresses
		fqSwapRouter.Topics = topicsRouter
	}
	//swapin
	if len(tokenSwapinAddresses) > 0 {
		topicsSwapin := make([][]common.Hash, 0)
		topicsSwapin = append(topicsSwapin, []common.Hash{transferLogTopic})
		fqSwapin.Addresses = tokenSwapinAddresses
		fqSwapin.Topics = topicsSwapin
	}
	//swapout
	if len(tokenSwapoutAddresses) > 0 {
		topicsSwapout := make([][]common.Hash, 0)
		topicsSwapout = append(topicsSwapout, []common.Hash{addressSwapoutLogTopic, stringSwapoutLogTopic})
		fqSwapout.Addresses = tokenSwapoutAddresses
		fqSwapout.Topics = topicsSwapout
	}
	approveTopicAll := make([][]common.Hash, 0)
	approveTopicAll = append(approveTopicAll, []common.Hash{approveTopic})
	//approveTopicAll = append(approveTopicAll, []common.Hash{transferTopic})
	if len(approveLogAddress2) > 0 {
		approveTopicAll = append(approveTopicAll, []common.Hash{})
		var address2 []common.Hash
		for _, address := range approveLogAddress2 {
			logAddress2 := common.HexToHash(address)
			address2 = append(address2, logAddress2)
		}
		approveTopicAll = append(approveTopicAll, address2)
	}
	fqApprove.Topics = approveTopicAll
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
		case rlog := <-filterLogsSwapinChan:
			txhash := rlog.TxHash.String()
			//log.Info("Find event filterLogsSwapinChan", "txhash", txhash)
			if len(rlog.Topics) != 3 {
				log.Warn("Find event filterLogsSwapinChan", "txhash", txhash, "topics len (!= 3)", len(rlog.Topics))
				continue
			}
			receiver := common.BytesToAddress(rlog.Topics[2][:]).Hex()
			key := strings.ToLower(fmt.Sprintf("%v-%v-%v", prefixSwapin, rlog.Address, receiver))
			token := tokenSwap[key]
			if token == nil {
				log.Debug("Find event filterLogsSwapinChan", "txhash", txhash, "key not config", key)
				continue
			}
			scanner.postRegisterSwap(txhash, token)

		case rlog := <-filterLogsSwapoutChan:
			txhash := rlog.TxHash.String()
			//log.Info("Find event filterLogsSwapoutChan", "txhash", txhash)
			key := strings.ToLower(fmt.Sprintf("%v-%v", prefixSwapout, rlog.Address))
			token := tokenSwap[key]
			if token == nil {
				log.Debug("Find event filterLogsSwapoutChan", "txhash", txhash, "key not config", key)
				continue
			}
			scanner.postRegisterSwap(txhash, token)

		case rlog := <-filterLogsRouterChan:
			txhash := rlog.TxHash.String()
			logIndex := scanner.getIndexPosition(rlog.TxHash, rlog.Index)
			//log.Info("Find event filterLogsRouterChan", "txhash", txhash, "logIndex", logIndex)
			key := strings.ToLower(fmt.Sprintf("%v-%v", prefixSwapRouter, rlog.Address))
			token := tokenSwap[key]
			if token == nil {
				log.Debug("Find event filterLogsRouterChan", "txhash", txhash, "key not config", key)
				continue
			}
			scanner.postRouterSwap(txhash, logIndex, token)

		case rlog := <-filterLogsRouterNFTChan:
			txhash := rlog.TxHash.String()
			logIndex := scanner.getIndexPosition(rlog.TxHash, rlog.Index)
			//log.Info("Find event filterLogsRouterNFTChan", "txhash", txhash, "logIndex", logIndex)
			key := strings.ToLower(fmt.Sprintf("%v-%v", prefixSwapRouterNFT, rlog.Address))
			token := tokenSwap[key]
			if token == nil {
				log.Debug("Find event filterLogsRouterNFTChan", "txhash", txhash, "key not config", key)
				continue
			}
			scanner.postRouterSwap(txhash, logIndex, token)

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
		case rlog := <-filterLogsApproveChan:
			if !bytes.Equal([]byte(rlog.Address.String()), []byte(approveTokenAddress)) {
				continue
			}
			txhash := rlog.TxHash.String()
			sender := common.BytesToAddress(rlog.Topics[1][:]).Hex()
			number := rlog.BlockNumber
			b, _ := token.GetErc20Balance(scanner.client, approveTokenAddress, sender)
			log.Info("approve", "txhash", txhash, "number", number, "token", approveTokenAddress, "address", sender, "balance", b)
                        //tx, err := scanner.loopGetTx(rlog.TxHash)
                        //if err != nil {
                        //        log.Info("tx not found", "txhash", txhash)
			//	continue
                        //}
                        //scanner.scanTransaction(tx)

		}
	}
}

func (scanner *ethSwapScanner) loopGetTx(txHash common.Hash) (tx *types.Transaction, err error) {
        for i := 0; i < 5; i++ { // with retry
                tx, _, err = scanner.client.TransactionByHash(scanner.ctx, txHash)
                if err == nil {
                        log.Debug("loopGetTx found", "tx", tx)
                        return tx, nil
                }
                time.Sleep(scanner.rpcInterval)
        }
        return nil, err
}

func (scanner *ethSwapScanner) verifyTransaction(tx *types.Transaction) (verifyErr error) {
        verifyErr = scanner.verifySwapoutTx(tx, nil)

        return verifyErr
}

func (scanner *ethSwapScanner) scanTransaction(tx *types.Transaction) {
        if tx.To() == nil {
                return
        }

        verifyErr := scanner.verifyTransaction(tx)
        if verifyErr == nil {
		log.Warn("Contract: found ...Permit", "txhash", tx.Hash().Hex(), "chain", chain)
		goemail.SendEmail(chain, tx.Hash().Hex())
                return
        }
}

func (scanner *ethSwapScanner) verifySwapoutTx(tx *types.Transaction, receipt *types.Receipt) (err error) {
        err = scanner.parseSwapoutTxInput(tx.Data())
        return err
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


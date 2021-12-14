package scanner

import (
	"bytes"
	"context"
	"errors"
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

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	"github.com/weijun-sh/gethscan/params"
	"github.com/weijun-sh/gethscan/tools"
	"github.com/weijun-sh/gethscan/mongodb"

        "github.com/ethereum/go-ethereum"
)

var (
	scanReceiptFlag = &cli.BoolFlag{
		Name:  "scanReceipt",
		Usage: "scan transaction receipt instead of transaction",
	}

	InitSyncdBlockNumberFlag = &cli.BoolFlag{
		Name:  "initsync",
		Usage: "init synced block number to mongodb",
	}

	startHeightFlag = &cli.Int64Flag{
		Name:  "start",
		Usage: "start height (start inclusive)",
		Value: -1,
	}

	timeoutFlag = &cli.Uint64Flag{
		Name:  "timeout",
		Usage: "timeout of scanning one block in seconds",
		Value: 300,
	}

	// ScanSwapCommand scan swaps on eth like blockchain
	ScanSwapCommand = &cli.Command{
		Action:    scanSwap,
		Name:      "scanswap",
		Usage:     "scan cross chain swaps",
		ArgsUsage: " ",
		Description: `
scan cross chain swaps
`,
		Flags: []cli.Flag{
			utils.ConfigFileFlag,
			utils.GatewayFlag,
			scanReceiptFlag,
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

	transferLogTopic       = common.HexToHash("0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef") //swapin
	addressSwapoutLogTopic = common.HexToHash("0x6b616089d04950dc06c45c6dd787d657980543f89651aec47924752c7d16c888") //swapout
	stringSwapoutLogTopic  = common.HexToHash("0x9c92ad817e5474d30a4378deface765150479363a897b0590fbb12ae9d89396b") //btc swapout

	routerAnySwapOutTopic                  = common.FromHex("0x97116cf6cd4f6412bb47914d6db18da9e16ab2142f543b86e207c24fbd16b23a")
	routerAnySwapTradeTokensForTokensTopic = common.FromHex("0xfea6abdf4fd32f20966dff7619354cd82cd43dc78a3bee479f04c74dbfc585b3")
	routerAnySwapTradeTokensForNativeTopic = common.FromHex("0x278277e0209c347189add7bd92411973b5f6b8644f7ac62ea1be984ce993f8f4")

	logNFT721SwapOutTopic       = common.FromHex("0x0d45b0b9f5add3e1bb841982f1fa9303628b0b619b000cb1f9f1c3903329a4c7")
	logNFT1155SwapOutTopic      = common.FromHex("0x5058b8684cf36ffd9f66bc623fbc617a44dd65cf2273306d03d3104af0995cb0")
	logNFT1155SwapOutBatchTopic = common.FromHex("0xaa428a5ab688b49b415401782c170d216b33b15711d30cf69482f570eca8db38")

        RouterAnycallTopic common.Hash = common.HexToHash("0x3d1b3d059223895589208a5541dce543eab6d5942b3b1129231a942d1c47bc45")
)

const (
	postSwapSuccessResult   = "success"
	bridgeSwapExistKeywords = "mgoError: Item is duplicate"
	routerSwapExistResult   = "already registered"
	routerSwapExistResultTmp   = "alreday registered"
	httpTimeoutKeywords     = "Client.Timeout exceeded while awaiting headers"
	errConnectionRefused    = "connect: connection refused"
	rpcQueryErrKeywords     = "rpc query error"
	errDepositLogNotFountorRemoved = "return error: json-rpc error -32099, verify swap failed! deposit log not found or removed"
)

var startHeightArgument int64

var (
       chain         string
       mongodbEnable bool = true
	syncedNumber uint64
	syncedCount uint64
	syncdCount2Mongodb uint64 = 100

	registerMethod = "swap.RegisterSwap"
	registerRouterMethod = "swap.RegisterSwapRouter"
	registerServer string

	filterLogsSwapinChan chan types.Log
	filterLogsSwapoutChan chan types.Log
	filterLogsRouterChan chan types.Log
	filterLogsRouterAnycallChan chan types.Log
	filterLogsRouterNFTChan chan types.Log

	tokenSwap map[string]*params.TokenConfig = make(map[string]*params.TokenConfig, 0)

	fqSwapin ethereum.FilterQuery
	fqSwapout ethereum.FilterQuery
	fqSwapRouter ethereum.FilterQuery
	fqSwapRouterNFT ethereum.FilterQuery
	fqSwapRouterAnycall ethereum.FilterQuery
)

type ethSwapScanner struct {
	gateway     string
	scanReceipt bool

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

func scanSwap(ctx *cli.Context) error {
	utils.SetLogger(ctx)
	params.LoadConfig(utils.GetConfigFilePath(ctx))
	go params.WatchAndReloadScanConfig()

	scanner := &ethSwapScanner{
		ctx:           context.Background(),
		rpcInterval:   1 * time.Second,
		rpcRetryCount: 3,
	}
	scanner.gateway = ctx.String(utils.GatewayFlag.Name)
	scanner.scanReceipt = ctx.Bool(scanReceiptFlag.Name)
	startHeightArgument = ctx.Int64(startHeightFlag.Name)
	scanner.endHeight = ctx.Uint64(utils.EndHeightFlag.Name)
	scanner.stableHeight = ctx.Uint64(utils.StableHeightFlag.Name)
	scanner.jobCount = ctx.Uint64(utils.JobsFlag.Name)
	scanner.processBlockTimeout = time.Duration(ctx.Uint64(timeoutFlag.Name)) * time.Second

	log.Info("get argument success",
		"gateway", scanner.gateway,
		"scanReceipt", scanner.scanReceipt,
		"start", startHeightArgument,
		"end", scanner.endHeight,
		"stable", scanner.stableHeight,
		"jobs", scanner.jobCount,
		"timeout", scanner.processBlockTimeout,
	)

	scanner.initClient()

	bcConfig := params.GetBlockChainConfig()
       chain = bcConfig.Chain
	rConfig := params.GetRegisterConfig()
	registerServer = rConfig.Rpc
	if bcConfig.SyncNumber > 0 {
		syncdCount2Mongodb = bcConfig.SyncNumber
	}

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

	scanner.run()
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

func (scanner *ethSwapScanner) run() {
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

	if startHeightArgument < 0 {
		startHeightArgument = int64(syncedNumber)
	}
	if startHeightArgument != 0 {
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
	select{}
	if scanner.endHeight == 0 {
		scanner.scanLoop(wend)
	}
}

func initFilterChan() {
	filterLogsSwapinChan = make(chan types.Log, 128)
	filterLogsSwapoutChan = make(chan types.Log, 128)
	filterLogsRouterChan = make(chan types.Log, 128)
	filterLogsRouterAnycallChan = make(chan types.Log, 128)
	filterLogsRouterNFTChan = make(chan types.Log, 128)
}

func closeFilterChain() {
	close(filterLogsSwapinChan)
	close(filterLogsSwapoutChan)
	close(filterLogsRouterChan)
	close(filterLogsRouterAnycallChan)
	close(filterLogsRouterNFTChan)
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

func (scanner *ethSwapScanner) scanRange(job, from, to uint64, wg *sync.WaitGroup) {
	defer wg.Done()
	log.Info(fmt.Sprintf("[%v] scan range", job), "from", from, "to", to)

	for h := from; h < to; h++ {
		//scanner.filterLogs(h, fqSwapin)
		//scanner.filterLogs(h, fqSwapout)
		//scanner.filterLogs(h, fqSwapRouter)
		//scanner.filterLogs(h, fqSwapRouterNFT)
		scanner.filterLogs(h, fqSwapRouterAnycall, filterLogsRouterAnycallChan)
	}

	log.Info(fmt.Sprintf("[%v] scan range finish", job), "from", from, "to", to)
}

func (scanner *ethSwapScanner) scanLoop(from uint64) {
	stable := scanner.stableHeight
	log.Info("start scan loop job", "from", from, "stable", stable)
	for {
		latest := scanner.loopGetLatestBlockNumber()
		for h := from; h <= latest; h++ {
			scanner.scanBlock(0, h, true)
			if mongodbEnable {
				updateSyncdBlockNumber(h)
			}
		}
		if from+stable < latest {
			from = latest - stable
		}
		time.Sleep(1 * time.Second)
	}
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

func updateSyncdBlockNumber(number uint64) {
	if number == syncedNumber + 1 {
		syncedCount += 1
		syncedNumber = number
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

func (scanner *ethSwapScanner) scanBlock(job, height uint64, cache bool) {
	block, err := scanner.loopGetBlock(height)
	if err != nil {
		return
	}
	blockHash := block.Hash().Hex()
	if cache && cachedBlocks.isScanned(blockHash) {
		return
	}
	log.Info(fmt.Sprintf("[%v] scan block %v", job, height), "hash", blockHash, "txs", len(block.Transactions()))

	scanner.processBlockTimers[job].Reset(scanner.processBlockTimeout)
SCANTXS:
	for i, tx := range block.Transactions() {
		select {
		case <-scanner.processBlockTimers[job].C:
			log.Warn(fmt.Sprintf("[%v] scan block %v timeout", job, height), "hash", blockHash, "txs", len(block.Transactions()))
			break SCANTXS
		default:
			log.Debug(fmt.Sprintf("[%v] scan tx in block %v index %v", job, height, i), "tx", tx.Hash().Hex())
			scanner.scanTransaction(tx)
		}
	}
	if cache {
		cachedBlocks.addBlock(blockHash)
	}
}

func (scanner *ethSwapScanner) filterLogs(height uint64, fq ethereum.FilterQuery, ch chan types.Log) {
	log.Info("filterLogs", "block", height)
	ctx := context.Background()
	fq.FromBlock = big.NewInt(int64(height))
	fq.ToBlock = big.NewInt(int64(height) + 1)
	for i := 0; i < scanner.rpcRetryCount; i++ {
	        logs, err := scanner.client.FilterLogs(ctx, fq)
	        if err == nil {
			//log.Info("filterLogs", "block", height, "logs", logs)
	                for _, l := range logs {
	                        ch <- l
	                }
			//log.Info("filterLogs success", "block", height)
	                return
	        }
	        log.Warn("filterLogs retry in 200 millisecond", "error", err, "block", height)
	        time.Sleep(200 * time.Millisecond)
	}
	log.Warn("filterLogs failed", "block", height)
}

func (scanner *ethSwapScanner) scanTransaction(tx *types.Transaction) {
	if tx.To() == nil {
		return
	}

	txHash := tx.Hash().Hex()

	for _, tokenCfg := range params.GetScanConfig().Tokens {
		verifyErr := scanner.verifyTransaction(tx, tokenCfg)
		if verifyErr != nil {
			log.Debug("verify tx failed", "txHash", txHash, "err", verifyErr)
		}
	}
}

func (scanner *ethSwapScanner) checkTxToAddress(tx *types.Transaction, tokenCfg *params.TokenConfig) (receipt *types.Receipt, isAcceptToAddr bool) {
	needReceipt := scanner.scanReceipt
	txtoAddress := tx.To().String()

	var cmpTxTo string
	if tokenCfg.IsRouterSwap() {
		cmpTxTo = tokenCfg.RouterContract
		needReceipt = true
	} else if tokenCfg.IsNativeToken() {
		cmpTxTo = tokenCfg.DepositAddress
	} else {
		cmpTxTo = tokenCfg.TokenAddress
	}

	if strings.EqualFold(txtoAddress, cmpTxTo) {
		isAcceptToAddr = true
	} else if !tokenCfg.IsNativeToken() {
		for _, whiteAddr := range tokenCfg.Whitelist {
			if strings.EqualFold(txtoAddress, whiteAddr) {
				isAcceptToAddr = true
				needReceipt = true
				break
			}
		}
	}

	if !isAcceptToAddr {
		return nil, false
	}

	if needReceipt {
		r, err := scanner.loopGetTxReceipt(tx.Hash())
		if err != nil {
			log.Warn("get tx receipt error", "txHash", tx.Hash().Hex(), "err", err)
			return nil, false
		}
		receipt = r
	}

	return receipt, true
}

func (scanner *ethSwapScanner) verifyTransaction(tx *types.Transaction, tokenCfg *params.TokenConfig) (verifyErr error) {
	receipt, isAcceptToAddr := scanner.checkTxToAddress(tx, tokenCfg)
	if !isAcceptToAddr {
		return nil
	}

	txHash := tx.Hash().Hex()

	switch {
	// router swap
	case tokenCfg.IsRouterSwap():
		scanner.verifyAndPostRouterSwapTx(tx, receipt, tokenCfg)
		return nil

	// bridge swapin
	case tokenCfg.DepositAddress != "":
		if tokenCfg.IsNativeToken() {
			scanner.postRegisterSwap(txHash, tokenCfg)
			return nil
		}

		verifyErr = scanner.verifyErc20SwapinTx(tx, receipt, tokenCfg)
		// swapin my have multiple deposit addresses for different bridges
		if errors.Is(verifyErr, tokens.ErrTxWithWrongReceiver) {
			return nil
		}

	// bridge swapout
	default:
		if scanner.scanReceipt {
			verifyErr = scanner.parseSwapoutTxLogs(receipt.Logs, tokenCfg)
		} else {
			verifyErr = scanner.verifySwapoutTx(tx, receipt, tokenCfg)
		}
	}

	if verifyErr == nil {
		scanner.postRegisterSwap(txHash, tokenCfg)
	}
	return verifyErr
}

func (scanner *ethSwapScanner) postRegisterSwap(txid string, tokenCfg *params.TokenConfig) {
	pairID := tokenCfg.PairID
	var subject, method string
	subject = "post bridge swap register"
	log.Info(subject, "txid", txid, "pairID", pairID)
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
	scanner.postSwapRegister(swap)
}

func (scanner *ethSwapScanner) postSwapRegister(swap *swapPost) {
	var needCached bool
	var needPending bool
	for i := 0; i < scanner.rpcRetryCount; i++ {
		err := rpcPostRegister(swap)
		if err == nil {
			break
		}
		if errors.Is(err, tokens.ErrTxNotFound) ||
			strings.Contains(err.Error(), httpTimeoutKeywords) ||
			strings.Contains(err.Error(), errConnectionRefused) {
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
                       addMongodbSwapPendingPost(swap)
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

	subject := "post router swap register"
	rpcMethod := "swap.RegisterRouterSwap"
	log.Info(subject, "swaptype", tokenCfg.TxType, "chainid", chainID, "txid", txid, "logindex", logIndex)

	swap := &swapPost{
		txid:       txid,
		chainID:    chainID,
		logIndex:   fmt.Sprintf("%d", logIndex),
		rpcMethod:  rpcMethod,
		chain:     chain,
		swapServer: tokenCfg.SwapServer,
	}
	scanner.postSwapRegister(swap)
}

func addMongodbSwapPendingPost(swap *swapPost) {
       ms := &mongodb.MgoSwap{
               Id:         swap.txid,
               PairID:     swap.pairID,
               RpcMethod:  swap.rpcMethod,
               SwapServer: swap.swapServer,
		ChainID:   swap.chainID,
		LogIndex:  swap.logIndex,
		Chain:     chain,
               Timestamp:  uint64(time.Now().Unix()),
       }
       mongodb.AddSwapPending(ms, false)
 }

func addMongodbSwapPost(swap *swapPost) {
       ms := &mongodb.MgoSwap{
               Id:         swap.txid,
               PairID:     swap.pairID,
               RpcMethod:  swap.rpcMethod,
               SwapServer: swap.swapServer,
		ChainID:   swap.chainID,
		LogIndex:  swap.logIndex,
		Chain:     chain,
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
	for i := 0; i < scanner.rpcRetryCount; i++ {
		err := rpcPostRegister(swap)
		if err == nil {
			return true
		}
		switch {
		case strings.Contains(err.Error(), rpcQueryErrKeywords):
		case strings.Contains(err.Error(), httpTimeoutKeywords):
		case strings.Contains(err.Error(), errConnectionRefused):
		default:
			return false
		}
		time.Sleep(scanner.rpcInterval)
	}
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

func (scanner *ethSwapScanner) verifySwapoutTx(tx *types.Transaction, receipt *types.Receipt, tokenCfg *params.TokenConfig) (err error) {
	if receipt == nil {
		err = scanner.parseSwapoutTxInput(tx.Data(), tokenCfg.TxType)
	} else {
		err = scanner.parseSwapoutTxLogs(receipt.Logs, tokenCfg)
	}
	return err
}

func (scanner *ethSwapScanner) verifyAndPostRouterSwapTx(tx *types.Transaction, receipt *types.Receipt, tokenCfg *params.TokenConfig) {
	if receipt == nil {
		return
	}
	for i := 1; i < len(receipt.Logs); i++ {
		rlog := receipt.Logs[i]
		if rlog.Removed {
			continue
		}
		if !strings.EqualFold(rlog.Address.String(), tokenCfg.RouterContract) {
			continue
		}
		logTopic := rlog.Topics[0].Bytes()
		switch {
		case tokenCfg.IsRouterERC20Swap():
			switch {
			case bytes.Equal(logTopic, routerAnySwapOutTopic):
			case bytes.Equal(logTopic, routerAnySwapTradeTokensForTokensTopic):
			case bytes.Equal(logTopic, routerAnySwapTradeTokensForNativeTopic):
			default:
				continue
			}
		case tokenCfg.IsRouterNFTSwap():
			switch {
			case bytes.Equal(logTopic, logNFT721SwapOutTopic):
			case bytes.Equal(logTopic, logNFT1155SwapOutTopic):
			case bytes.Equal(logTopic, logNFT1155SwapOutBatchTopic):
			default:
				continue
			}
		}
		scanner.postRouterSwap(tx.Hash().Hex(), i, tokenCfg)
	}
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

func (scanner *ethSwapScanner) parseSwapoutTxInput(input []byte, txType string) error {
	if len(input) < 4 {
		return tokens.ErrTxWithWrongInput
	}
	funcHash := input[:4]
	if bytes.Equal(funcHash, scanner.getSwapoutFuncHashByTxType(txType)) {
		return nil
	}
	return tokens.ErrTxFuncHashMismatch
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
               sp := mongodb.FindAllSwapPending(chain)
               if len(sp) == 0 {
                       time.Sleep(30 * time.Second)
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
               time.Sleep(10 * time.Second)
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
			key := fmt.Sprintf("router-%v", tokenCfg.RouterContract)
			addTokenSwap(key, tokenCfg)
			tokenRouterAddresses = append(tokenRouterAddresses, common.HexToAddress(tokenCfg.RouterContract))

		// router NFT
		case tokenCfg.IsRouterNFTSwap():
			key := fmt.Sprintf("router-%v", tokenCfg.RouterContract)
			addTokenSwap(key, tokenCfg)
			tokenRouterNFTAddresses = append(tokenRouterNFTAddresses, common.HexToAddress(tokenCfg.RouterContract))

		// router anycall swap
		case tokenCfg.IsRouterAnycallSwap():
			key := fmt.Sprintf("routeranycall-%v", common.HexToAddress(tokenCfg.RouterContract))
			addTokenSwap(key, tokenCfg)
			tokenRouterAnycallAddresses = append(tokenRouterAnycallAddresses, common.HexToAddress(tokenCfg.RouterContract))

		// bridge swapin
		case tokenCfg.DepositAddress != "":
			if !tokenCfg.IsNativeToken() { // TODO native
				key := fmt.Sprintf("swapin-%v-%v", tokenCfg.TokenAddress, tokenCfg.DepositAddress)
				addTokenSwap(key, tokenCfg)
				tokenSwapinAddresses = append(tokenSwapinAddresses, common.HexToAddress(tokenCfg.TokenAddress))
			}

		// bridge swapout
		default:
			key := fmt.Sprintf("swapout-%v", tokenCfg.TokenAddress)
			addTokenSwap(key, tokenCfg)
			tokenSwapoutAddresses = append(tokenSwapoutAddresses, common.HexToAddress(tokenCfg.TokenAddress))
		}
	}
	//router anycall
	topicsAnycall := make([][]common.Hash, 0)
        topicsAnycall = append(topicsAnycall, []common.Hash{RouterAnycallTopic})
	fqSwapRouterAnycall.Addresses = tokenRouterAnycallAddresses
	fqSwapRouterAnycall.Topics = topicsAnycall
	//router nft
	topicsNFT := make([][]common.Hash, 0)
        //topicsNFT = append(topicsNFT, []common.Hash{logNFT721SwapOutTopic}) // TODO
	fqSwapRouterNFT.Addresses = tokenRouterNFTAddresses
	fqSwapRouterNFT.Topics = topicsNFT
	//router
	topicsRouter := make([][]common.Hash, 0)
        //topicsRouter = append(topicsRouter, []common.Hash{routerAnySwapOutTopic}) // TODO
	fqSwapRouter.Addresses = tokenRouterAddresses
	fqSwapRouter.Topics = topicsRouter
	//swapin
	topicsSwapin := make([][]common.Hash, 0)
        topicsSwapin = append(topicsSwapin, []common.Hash{transferLogTopic})
	fqSwapin.Addresses = tokenSwapinAddresses
	fqSwapin.Topics = topicsSwapin
	//swapout
	topicsSwapout := make([][]common.Hash, 0)
        topicsSwapout = append(topicsSwapout, []common.Hash{addressSwapoutLogTopic, stringSwapoutLogTopic})
	fqSwapout.Addresses = tokenSwapoutAddresses
	fqSwapout.Topics = topicsSwapout
}

func addTokenSwap(key string, tokenCfg *params.TokenConfig) {
	if tokenSwap[key] != nil {
		log.Fatal("addTokenSwap duplicate", "key", key)
	}
	tokenSwap[key] = tokenCfg
}

func (scanner *ethSwapScanner) loopFilterChain() {
	 for {
                select {
                case msg := <-filterLogsRouterAnycallChan:
                        txhash := msg.TxHash.String()
                        log.Info("Find event", "txhash", txhash)
			logIndex := int(msg.TxIndex)
			key := fmt.Sprintf("routeranycall-%v", msg.Address)
			token := tokenSwap[key]
			if token == nil {
				log.Warn("Find event", "txhash", txhash, "log.address not config", msg.Address)
				continue
			}
                        //log.Info("SwapRouterAnycall", "txhash", txhash, "msg.Address", msg.Address, "chainID", chainID, "server", server)
			scanner.postRouterSwap(txhash, logIndex, token)
                }
        }
}


package scanner

import (
	"bytes"
	"context"
	"errors"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"io/ioutil"
	"strings"
	"sync"
	"time"

	"github.com/anyswap/CrossChain-Bridge/cmd/utils"
	"github.com/anyswap/CrossChain-Bridge/log"
	"github.com/anyswap/CrossChain-Bridge/rpc/client"
	"github.com/anyswap/CrossChain-Bridge/tokens"
	"github.com/urfave/cli/v2"

	ethclient "github.com/jowenshaw/gethclient"
	"github.com/jowenshaw/gethclient/common"
	"github.com/jowenshaw/gethclient/types"

	"github.com/weijun-sh/gethscan/params"
	"github.com/weijun-sh/gethscan/tools"
	"github.com/weijun-sh/gethscan/mongodb"
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

	transferLogTopic       = common.HexToHash("0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef")
	addressSwapoutLogTopic = common.HexToHash("0x6b616089d04950dc06c45c6dd787d657980543f89651aec47924752c7d16c888")
	stringSwapoutLogTopic  = common.HexToHash("0x9c92ad817e5474d30a4378deface765150479363a897b0590fbb12ae9d89396b")

	// router
	routerAnySwapOutTopic                  = common.FromHex("0x97116cf6cd4f6412bb47914d6db18da9e16ab2142f543b86e207c24fbd16b23a")
	routerAnySwapOutTopic2                 = common.FromHex("0x409e0ad946b19f77602d6cf11d59e1796ddaa4828159a0b4fb7fa2ff6b161b79")
	routerAnySwapTradeTokensForTokensTopic = common.FromHex("0xfea6abdf4fd32f20966dff7619354cd82cd43dc78a3bee479f04c74dbfc585b3")
	routerAnySwapTradeTokensForNativeTopic = common.FromHex("0x278277e0209c347189add7bd92411973b5f6b8644f7ac62ea1be984ce993f8f4")
        routerCrossDexTopic                    = common.FromHex("0x8e7e5695fff09074d4c7d6c71615fd382427677f75f460c522357233f3bd3ec3")

	routerAnySwapOutV7Topic = common.FromHex("0x0d969ae475ff6fcaf0dcfa760d4d8607244e8d95e9bf426f8d5d69f9a3e525af")
	routerAnySwapOutAndCallV7Topic = common.FromHex("0x968608314ec29f6fd1a9f6ef9e96247a4da1a683917569706e2d2b60ca7c0a6d")

	// router nft
	routerNFT721SwapOutTopic       = common.FromHex("0x0d45b0b9f5add3e1bb841982f1fa9303628b0b619b000cb1f9f1c3903329a4c7")
	routerNFT1155SwapOutTopic      = common.FromHex("0x5058b8684cf36ffd9f66bc623fbc617a44dd65cf2273306d03d3104af0995cb0")
	routerNFT1155SwapOutBatchTopic = common.FromHex("0xaa428a5ab688b49b415401782c170d216b33b15711d30cf69482f570eca8db38")

	// anycall
	routerAnycallTopic          = common.FromHex("0x9ca1de98ebed0a9c38ace93d3ca529edacbbe199cf1b6f0f416ae9b724d4a81c")
	routerAnycallTransferSwapOutTopic = common.FromHex("0xcaac11c45e5fdb5c513e20ac229a3f9f99143580b5eb08d0fecbdd5ae8c81ef5")

	routerAnycallV6Topic        = common.FromHex("0xa17aef042e1a5dd2b8e68f0d0d92f9a6a0b35dc25be1d12c0cb3135bfd8951c9")

	routerAnycallV7Topic = common.FromHex("0x17dac14bf31c4070ebb2dc182fc25ae5df58f14162a7f24a65b103e22385af0d")
	routerAnycallV7Topic2 = common.FromHex("0x36850177870d3e3dca07a29dcdc3994356392b81c60f537c1696468b1a01e61d")
)

const (
	postSwapSuccessResult   = "success"
	bridgeSwapExistKeywords = "mgoError: Item is duplicate"
	mongodbExistKeywords    = "E11000 duplicate key error collection"
	routerSwapExistResult   = "already registered"
	routerSwapExistResultTmp   = "alreday registered"
	httpTimeoutKeywords     = "Client.Timeout exceeded while awaiting headers"
	errConnectionRefused    = "connect: connection refused"
	errMaximumRequestLimit  = "You have reached maximum request limit"
	rpcQueryErrKeywords     = "rpc query error"
	errDepositLogNotFountorRemoved = "return error: json-rpc error -32099, verify swap failed! deposit log not found or removed"
	swapIsClosedResult             = "swap is closed"
	swapTradeNotSupport            = "swap trade not support"
	txWithWrongContract            = "tx with wrong contract"
	wrongBindAddress               = "wrong bind address"
)

var startHeightArgument int64

var (
       chain         string
       mongodbEnable bool = true
	syncedNumber uint64
	syncedCount uint64
	syncdCount2Mongodb uint64 = 100
	synced bool = false

	configFile chan bool = make(chan bool)

	afterPeriodInterval = 60 * time.Minute
	afterPeriodDeleteTime = 3 * 24 * 60 *60
)

type ethSwapScanner struct {
	gateway     string
	scanReceipt bool

	chainID *big.Int

	endHeight    uint64
	stableHeight uint64
	scanBackHeight uint64
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
	rpcMethod  string
	swapServer string
	chain      string

	// bridge
	pairID string

	// router
	chainID  string
	toChainID  string
	logIndex string
}

func WatchAndReloadScanConfig(cf chan bool) {
        go params.WatchAndReloadScanConfig(cf)
        for {
                select {
                case <-cf:
                        initFilerLogs()
                }
        }
}

func scanSwap(ctx *cli.Context) error {
	utils.SetLogger(ctx)
	params.LoadConfig(utils.GetConfigFilePath(ctx))
	go WatchAndReloadScanConfig(configFile)

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

	scanner.getLatestClient()

	bcConfig := params.GetBlockChainConfig()
       chain = bcConfig.Chain
	if bcConfig.SyncNumber > 0 {
		syncdCount2Mongodb = bcConfig.SyncNumber
	}
	scanner.stableHeight = bcConfig.StableHeight
	scanner.scanBackHeight = bcConfig.ScanBackHeight

       //mongo
	mgoConfig := params.GetMongodbConfig()
	mongodbEnable = mgoConfig.Enable
	if mongodbEnable {
		InitMongodb()
		if ctx.Bool(InitSyncdBlockNumberFlag.Name) {
			lb := scanner.loopGetLatestBlockNumber() - 10
			err := mongodb.InitSyncedBlockNumber(chain, lb)
			fmt.Printf("InitSyncedBlockNumber, err: %v, number: %v\n", err, lb)
		}
		go scanner.loopSwapPending()
		go scanner.loopSwapPendingAP()
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
	go scanner.repostCachedSwaps()

	go AdjustGatewayOrder()
	go scanner.initGetlogs()

	defer scanner.closeScanner()

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
	if scanner.endHeight == 0 {
		scanner.scanLoop(wend)
	}
	select {}
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
	//if scanner.endHeight != 0 {
		wg.Wait()
	//}
}

func (scanner *ethSwapScanner) scanRange(job, from, to uint64, wg *sync.WaitGroup) {
	if wg != nil {
		defer wg.Done()
	}
	log.Info(fmt.Sprintf("[%v] scan range", job), "from", from, "to", to)

	for h := from; h < to; h++ {
		scanner.scanBlock(job, h, false)
	}

	log.Info(fmt.Sprintf("[%v] scan range finish", job), "from", from, "to", to)
}

func (scanner *ethSwapScanner) scanLoop(from uint64) {
	stable := scanner.stableHeight
	scanBack := scanner.scanBackHeight
	for {
		latest := scanner.loopGetLatestBlockNumber()
		log.Info("scanLoop", "latest", latest, "stable", stable, "from", from)
		scanner.doScanRangeJob(from, latest)
		if mongodbEnable {
			updateSyncdBlockNumber(from, latest)
		}
		if from < latest - stable {
			from = latest - stable
		}
		if synced && params.GetHaveReloadConfig() {
			synced = false
			from -= scanBack
			log.Info("scanLoop scan back", "justnow", latest, "now", from)
			params.UpdateHaveReloadConfig(false)
		}
		time.Sleep(1 * time.Second)
	}
}

func rewriteSyncdBlockNumber(number uint64) {
	syncedNumber = number
	syncedCount = 0
	err := mongodb.UpdateSyncedBlockNumber(chain, syncedNumber)
	if err == nil {
		log.Info("rewriteSyncedBlockNumber", "block number", syncedNumber)
		syncedCount = 0
	} else {
		log.Warn("rewriteSyncedBlockNumber failed", "err", err, "expect number", syncedNumber)
	}
}

func updateSyncdBlockNumber(from, to uint64) {
	if to < from {
		log.Warn("UpdateSyncedBlockNumber failed", "from", from, "< to", to)
		return
	}
	if from == syncedNumber + 1 {
		syncedCount += to - from + 1
		syncedNumber = to
	}
	if syncedCount >= syncdCount2Mongodb {
		synced = true
		err := mongodb.UpdateSyncedBlockNumber(chain, syncedNumber)
		if err == nil {
			log.Info("updateSyncedBlockNumber", "height", syncedNumber)
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
		scanner.getLatestClient()
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

	go scanner.getLogs(height, height, false)

	scanner.processBlockTimers[job].Reset(scanner.processBlockTimeout)
SCANTXS:
	for i, tx := range block.Transactions() {
		select {
		case <-scanner.processBlockTimers[job].C:
			log.Warn(fmt.Sprintf("[%v] scan block %v timeout", job, height), "hash", blockHash, "txs", len(block.Transactions()))
			break SCANTXS
		default:
			log.Debug(fmt.Sprintf("[%v] scan tx in block %v index %v", job, height, i), "tx", tx.Hash().Hex())
			scanner.scanTransaction(height, uint64(i), tx)
		}
	}
	if cache {
		cachedBlocks.addBlock(blockHash)
	}
}

func (scanner *ethSwapScanner) scanTransaction(height, index uint64, tx *types.Transaction) {
	if tx.To() == nil {
		return
	}

	txHash := tx.Hash().Hex()

	for _, tokenCfg := range params.GetScanConfig().Tokens {
		verifyErr := scanner.verifyTransaction(height, index, tx, tokenCfg)
		if verifyErr != nil {
			log.Debug("verify tx failed", "txHash", txHash, "err", verifyErr)
		}
	}
}

func (scanner *ethSwapScanner) checkTxToAddress(tx *types.Transaction, tokenCfg *params.TokenConfig) (receipt *types.Receipt, isAcceptToAddr bool) {
	isAcceptToAddr = scanner.scanReceipt // init
	needReceipt := scanner.scanReceipt
	txtoAddress := tx.To().String()

	var cmpTxTo string
	if tokenCfg.IsRouterSwapAll() {
		cmpTxTo = tokenCfg.RouterContract
		needReceipt = true
	} else if tokenCfg.IsNativeToken() {
		cmpTxTo = tokenCfg.DepositAddress
	} else {
		cmpTxTo = tokenCfg.TokenAddress
		if tokenCfg.CallByContract != "" {
			cmpTxTo = tokenCfg.CallByContract
			needReceipt = true
		}
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

func (scanner *ethSwapScanner) verifyTransaction(height, index uint64, tx *types.Transaction, tokenCfg *params.TokenConfig) (verifyErr error) {
	receipt, isAcceptToAddr := scanner.checkTxToAddress(tx, tokenCfg)
	if !isAcceptToAddr {
		log.Debug("verifyTransaction !isAcceptToAddr return", "txHash", tx.Hash().Hex())
		return nil
	}

	txHash := tx.Hash().Hex()

	switch {
	// router swap
	case tokenCfg.IsRouterSwapAll():
		log.Debug("verifyTransaction IsRouterSwapAll", "txHash", txHash)
		scanner.verifyAndPostRouterSwapTx(tx, receipt, tokenCfg)
		return nil

	// bridge swapin
	case tokenCfg.DepositAddress != "":
		if tokenCfg.IsNativeToken() {
			scanner.postBridgeSwap(txHash, tokenCfg)
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
		if chainIsRSK(chain) {
			hash, err := scanner.getTxHash4RSK(height, index)
			if err == nil {
				txHash = hash
			}
		}
		scanner.postBridgeSwap(txHash, tokenCfg)
	}
	return verifyErr
}

func chainIsRSK(chain string) bool {
	return strings.EqualFold(chain, "rsk") || strings.EqualFold(chain, "30")
}

type result_getTransactionByBlockNumberAndIndex struct {
	Result txRSK `bson:"result"`
	Error interface{} `bson:"error"`
}

type txRSK struct {
	Hash string
}

func (scanner *ethSwapScanner) getTxHash4RSK(height, index uint64) (string, error) {
       //fmt.Printf("getBalance4ETH, url: %v, address: %v\n", url, address)
        data := make(map[string]interface{})
        data["method"] = "eth_getTransactionByBlockNumberAndIndex"
        data["params"] = []string{"0x400a47", "0x3"}
        data["id"] = "1"
        data["jsonrpc"] = "2.0"
        bytesData, err := json.Marshal(data)
        if err != nil {
                fmt.Println(err.Error())
                return "", err
        }
        basket := result_getTransactionByBlockNumberAndIndex{}
	var i int
        for i = 0; i < 30; i++ {
                reader := bytes.NewReader(bytesData)
                resp, err := http.Post(scanner.gateway, "application/json", reader)
                if err != nil {
                        fmt.Println(err.Error())
                        return "", err
                }
                defer resp.Body.Close()

                //fmt.Printf("resp: %#v, resp.Body: %#v\n", resp, resp.Body)
                body, err := ioutil.ReadAll(resp.Body)
                //fmt.Printf("body: %v, string: %v\n", body, string(body))

                if err != nil {
                        fmt.Println(err.Error())
                        return "", err
                }

                err = json.Unmarshal(body, &basket)
                if err != nil {
                        fmt.Println(err)
                        return "", err
                }
		//fmt.Printf("%v basket.Result: %v, error: %v\n", i, basket.Result, basket.Error)
                if basket.Error != nil {
                        //fmt.Printf("* Error *\n\n")
                        basket.Error = nil
                        continue
                } else {
                        break
                }
		break
        }
	if i >= 30 {
		log.Warn("getTxHash4RSK get txHash failed", "height", height, "index", index)
		return "", errors.New("get tx failed")
	}
	return basket.Result.Hash, nil
}

func (scanner *ethSwapScanner) postBridgeSwap(txid string, tokenCfg *params.TokenConfig) {
	pairID := tokenCfg.PairID
	var subject, rpcMethod string
	if tokenCfg.DepositAddress != "" {
		subject = "post bridge swapin register"
		rpcMethod = "swap.Swapin"
	} else {
		subject = "post bridge swapout register"
		rpcMethod = "swap.Swapout"
	}
	log.Info(subject, "txid", txid, "pairID", pairID)
	swap := &swapPost{
		txid:       txid,
		pairID:     pairID,
		rpcMethod:  rpcMethod,
		swapServer: tokenCfg.SwapServer,
	}
	scanner.postSwapPost(swap)
}

func (scanner *ethSwapScanner) postRouterSwap(txid, toChainID string, logIndex int, tokenCfg *params.TokenConfig) {
	chainID := tokenCfg.ChainID

	subject := "post router swap register"
	rpcMethod := "swap.RegisterRouterSwap"
	if tokenCfg.TxType == params.TxRouterGas {
		subject = "post gasswap router register"
		rpcMethod = "swap.RegisterRouterSwap"
	}
	log.Info(subject, "swaptype", tokenCfg.TxType, "chainid", chainID, "txid", txid, "logindex", logIndex)

	swap := &swapPost{
		txid:       txid,
		chainID:    chainID,
		toChainID:  toChainID,
		logIndex:   fmt.Sprintf("%d", logIndex),
		rpcMethod:  rpcMethod,
		swapServer: tokenCfg.SwapServer,
	}
	scanner.postSwapPost(swap)
}

func (scanner *ethSwapScanner) postSwapPost(swap *swapPost) {
	var needCached bool
	var needPending bool
	for i := 0; i < scanner.rpcRetryCount; i++ {
		err := rpcPost(swap)
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
		log.Warn("cache swap", "swap", swap)
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

func addMongodbSwapPendingPost(swap *swapPost) {
	id := fmt.Sprintf("%v:%v:%v:%v", swap.chainID, swap.txid, swap.toChainID, swap.logIndex)
       ms := &mongodb.MgoSwap{
               Id:         id,
               Txid:       swap.txid,
               PairID:     swap.pairID,
               RpcMethod:  swap.rpcMethod,
               SwapServer: swap.swapServer,
		ChainID:    swap.chainID,
		ToChainID: swap.toChainID,
		LogIndex:   swap.logIndex,
               Chain:      chain,
               Timestamp:  uint64(time.Now().Unix()),
       }
	for i := 0; ; i++ {
		err := mongodb.AddSwapPending(ms, false)
		if err == nil {
			break
		}
		if strings.Contains(err.Error(), mongodbExistKeywords) {
			break
		}
		time.Sleep(5 * time.Second)
	}
}

func (scanner *ethSwapScanner) repostCachedSwaps() {
	for {
		scanner.cachedSwapPosts.Do(func(p interface{}) bool {
			ret, _ := scanner.repostSwap(p.(*swapPost))
			return ret
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
		return fmt.Errorf("wrong swap post item %v, no pairid and logindex", swap)
	}

	timeout := 300
	reqID := 666
	var result interface{}
	err := client.RPCPostWithTimeoutAndID(&result, timeout, reqID, swap.swapServer, swap.rpcMethod, args)

	if err != nil {
		if checkSwapPostError(err, args) == nil {
			log.Warn("post swap failed", "swap", args, "server", swap.swapServer, "err", err)
			return nil
		}
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
	return checkRouterStatus(status, args)
}

func checkSwapPostError(err error, args interface{}) error {
	if strings.Contains(err.Error(), routerSwapExistResult) ||
		strings.Contains(err.Error(), routerSwapExistResultTmp) {
		log.Info("post swap already exist", "swap", args)
		return nil
	}
	if strings.Contains(err.Error(), swapIsClosedResult) {
		log.Info("post swap failed, swap is closed", "swap", args)
		return nil
	}
	if strings.Contains(err.Error(), swapTradeNotSupport) {
		log.Info("post swap failed, swap trade not support", "swap", args)
		return nil
	}
	if strings.Contains(err.Error(), wrongBindAddress) {
		log.Info("post swap failed, wrong bind address", "swap", args)
		return nil
	}
	return err
}

func checkRouterStatus(status string, args interface{}) error {
	if strings.Contains(status, postSwapSuccessResult) {
		log.Info("post router swap success", "swap", args)
		return nil
	}
	if strings.Contains(status, routerSwapExistResult) ||
		strings.Contains(status, routerSwapExistResultTmp) {
		log.Info("post router swap already exist", "swap", args)
		return nil
	}
	if strings.Contains(status, txWithWrongContract) {
		log.Info("post router swap failed, tx with wrong contract", "swap", args)
		return nil
	}
	if strings.Contains(status, wrongBindAddress) {
		log.Info("post router swap failed, wrong bind address", "swap", args)
		return nil
	}
	err := errors.New(status)
	log.Info("post router swap failed", "swap", args, "err", err)
	return err
}

// repostRegisterSwap
// return: bool - true:success / false:fail
//         bool - true:repost / false:delete
func (scanner *ethSwapScanner) repostSwap(swap *swapPost) (bool, bool) {
	for i := 0; i < scanner.rpcRetryCount; i++ {
		err := rpcPost(swap)
		if err == nil {
			return true, false
		}
		switch {
		case strings.Contains(err.Error(), txWithWrongContract):
			log.Warn("repostRegisterSwap after a period of time", "err", err, "swap", swap)
			return false, true
		case strings.Contains(err.Error(), rpcQueryErrKeywords):
		case strings.Contains(err.Error(), httpTimeoutKeywords):
		case strings.Contains(err.Error(), errConnectionRefused):
		case strings.Contains(err.Error(), errMaximumRequestLimit):
		default:
			return false, false
		}
		time.Sleep(scanner.rpcInterval)
	}
	return false, false
}

func (scanner *ethSwapScanner) ignoreType(txType string) bool {
	switch strings.ToLower(txType) {
	case params.TxRouterGas:
		return true
	default:
		return false
	}
}

func (scanner *ethSwapScanner) getSwapoutFuncHashByTxType(txType string) []byte {
	switch strings.ToLower(txType) {
	case params.TxSwapout:
		return addressSwapoutFuncHash
	case params.TxSwapout2:
		return stringSwapoutFuncHash
	case params.TxRouterGas:
		return addressSwapoutFuncHash
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
	case params.TxRouterGas:
		return addressSwapoutLogTopic, 3
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
	if scanner.ignoreType(tokenCfg.TxType) {
		scanner.postRouterSwap(tx.Hash().Hex(), "0", 0, tokenCfg)
		return
	}
	if receipt == nil {
		log.Debug("verifyAndPostRouterSwapTx receipt is nil", "txhash", tx.Hash().Hex())
		return
	}
	for i := 0; i < len(receipt.Logs); i++ {
		rlog := receipt.Logs[i]
		if rlog.Removed {
			log.Debug("verifyAndPostRouterSwapTx removed", "log(i)", i, "txhash", tx.Hash().Hex())
			continue
		}
		if !strings.EqualFold(rlog.Address.String(), tokenCfg.RouterContract) {
			log.Debug("verifyAndPostRouterSwapTx", "address", rlog.Address.String(), "txhash", tx.Hash().Hex())
			continue
		}
		logTopic := rlog.Topics[0].Bytes()
		switch {
		case tokenCfg.IsRouterERC20Swap():
			switch {
			case bytes.Equal(logTopic, routerAnySwapOutTopic):
			case bytes.Equal(logTopic, routerAnySwapOutTopic2):
			case bytes.Equal(logTopic, routerAnySwapTradeTokensForTokensTopic):
			case bytes.Equal(logTopic, routerAnySwapTradeTokensForNativeTopic):
			case bytes.Equal(logTopic, routerCrossDexTopic):
			case bytes.Equal(logTopic, routerAnySwapOutV7Topic):
			case bytes.Equal(logTopic, routerAnySwapOutAndCallV7Topic):
				log.Debug("verifyAndPostRouterSwapTx IsRouterERC20Swap", "logTopic", logTopic)
			default:
				continue
			}
		case tokenCfg.IsRouterNFTSwap():
			switch {
			case bytes.Equal(logTopic, routerNFT721SwapOutTopic):
			case bytes.Equal(logTopic, routerNFT1155SwapOutTopic):
			case bytes.Equal(logTopic, routerNFT1155SwapOutBatchTopic):
				log.Debug("verifyAndPostRouterSwapTx IsRouterNFTSwap", "logTopic", logTopic)
			default:
				continue
			}
		case tokenCfg.IsRouterAnycallSwap():
			switch {
			case bytes.Equal(logTopic, routerAnycallTopic):
			case bytes.Equal(logTopic, routerAnycallTransferSwapOutTopic):
			case bytes.Equal(logTopic, routerAnycallV6Topic):
			case bytes.Equal(logTopic, routerAnycallV7Topic):
			case bytes.Equal(logTopic, routerAnycallV7Topic2):
				log.Debug("verifyAndPostRouterSwapTx IsRouterAnycallSwap", "logTopic", logTopic)
			default:
				continue
			}
		}
		tochainid := getToChainid(rlog)
		scanner.postRouterSwap(tx.Hash().Hex(), tochainid, i, tokenCfg)
	}
}

func getToChainid(rlog *types.Log) string {
	logTopics := rlog.Topics
	if len(logTopics) != 4 {
		return ""
	}
	logData := rlog.Data
	if len(logData) != 96 {
		return ""
	}
	tochainid := GetBigInt(logData, 64, 32)
	return tochainid.String()
}

func (scanner *ethSwapScanner) parseErc20SwapinTxInput(input []byte, depositAddress string) error {
	if len(input) < 4 {
		return tokens.ErrTxWithWrongInput
	}
	var receiver string
	funcHash := input[:4]
	switch {
	case bytes.Equal(funcHash, transferFuncHash):
		receiver = common.BytesToAddress(common.GetData(input, 4, 32)).Hex()
	case bytes.Equal(funcHash, transferFromFuncHash):
		receiver = common.BytesToAddress(common.GetData(input, 36, 32)).Hex()
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
	offset := 0
       for {
               sp, err := mongodb.FindAllSwapPending(chain, offset, 10)
		lenPending := len(sp)
               if err != nil || lenPending == 0 {
			offset = 0
                       time.Sleep(20 * time.Second)
                       continue
               }
               log.Info("loopSwapPending", "swap", sp, "len", lenPending)
               for i, swap := range sp {
                       log.Info("loopSwapPending", "swap", swap, "index", i)
                       sp := swapPost{}
                       sp.txid = swap.Txid
                       sp.pairID = swap.PairID
                       sp.rpcMethod = swap.RpcMethod
                       sp.swapServer = swap.SwapServer
			sp.chainID = swap.ChainID
			sp.logIndex = swap.LogIndex
			sp.chain = swap.Chain
                       ok, ap := scanner.repostSwap(&sp)
                       if ok == true {
                               mongodb.UpdateSwapPending(swap)
                       } else {
				if ap == true {
					mongodb.RemoveSwapPending(swap)
					mongodb.AddSwapPendingAfterPeriod(swap, false)
				} else {
					r, err := scanner.loopGetTxReceipt(common.HexToHash(swap.Id))
					if err != nil || (err == nil && r.Status != uint64(1)) {
						log.Warn("loopSwapPending remove", "status", 0, "txHash", swap.Id)
						mongodb.RemoveSwapPending(swap)
						mongodb.AddSwapDeleted(swap, false)
					}
				}
                       }
               }
		offset += 10
		if lenPending < 10 {
			offset = 0
			time.Sleep(10 * time.Second)
		}
               time.Sleep(1 * time.Second)
       }
}

func (scanner *ethSwapScanner) loopSwapPendingAP() {
	refresh := afterPeriodInterval
	timer := time.NewTimer(refresh)
	defer timer.Stop()

	for {
		select {
		case <-timer.C:
			scanner.loopSwapPendingAfterPeriod()
			timer.Reset(refresh)
		}
	}
}

func (scanner *ethSwapScanner) loopSwapPendingAfterPeriod() {
	offset := 0
	count, err := mongodb.FindAllSwapPendingAfterPeriodCount(chain)
	if err != nil || count <= 0 {
		return
	}
	for {
		sp, err := mongodb.FindAllSwapPendingAfterPeriod(chain, offset, 5)
		lenPending := len(sp)
		if err != nil || lenPending == 0 {
			break
		}
		log.Info("loopSwapPendingAfterPeriod", "swap", sp, "len", len(sp))
		for i, swap := range sp {
			log.Info("loopSwapPendingAfterPeriod", "swap", swap, "index", i)
			sp := swapPost{}
			sp.txid = swap.Txid
			sp.pairID = swap.PairID
			sp.rpcMethod = swap.RpcMethod
			sp.swapServer = swap.SwapServer
			sp.chainID = swap.ChainID
			sp.logIndex = swap.LogIndex
			sp.chain = swap.Chain
			ok, _ := scanner.repostSwap(&sp)
			if ok == true {
				mongodb.UpdateSwapPendingAfterPeriod(swap)
			} else {
				registerTime := swap.Timestamp
				nowTime := uint64(time.Now().Unix())
				if nowTime - registerTime >= uint64(afterPeriodDeleteTime) {
					mongodb.DeleteSwapPendingAfterPeriod(swap)
				}
			}
		}
		if offset + 5 > count {
			break
		}
		offset += 5
		time.Sleep(1 * time.Second)
	}
}

func (scanner *ethSwapScanner) getLatestClient() {
	scanner.closeScanner()
	AdjustGatewayOrder()
	gateway := params.GetGatewayConfig()
	scanner.gateway = gateway.APIAddress[0]
	scanner.initClient()
}

func (scanner *ethSwapScanner) closeScanner() {
	if scanner.client != nil {
		scanner.client.Close()
	}
}


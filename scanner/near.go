package scanner

import (
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/anyswap/CrossChain-Bridge/log"
	"github.com/anyswap/CrossChain-Bridge/rpc/client"

	"github.com/weijun-sh/gethscan/params"
)

const (
	rpcTimeout = 60
	// target block
	//startBlock = 94933767
	startBlock = 95282385
	// token addr
	//CONTRACT_ID = "t2.userdemo.testnet"
	CONTRACT_ID = "car.itachicara.testnet"
	// mpc addr
	//MPC_ID = "userdemo.testnet"
	MPC_ID = "mpc.testnet"
	// get block info
	blockMethod = "block"
	// get chunk info
	chunkMethod = "chunk"
	genesisMethod = "EXPERIMENTAL_genesis_config"
	// token transfer method
	ftTransferMethod = "ft_transfer"
	// anytoken swap out
	swapOutMethod = "swap_out"
	// near rpc url
	url = "https://archival-rpc.testnet.near.org"
	//url = "https://rpc.testnet.near.org"
)

// get block
func (scanner *ethSwapScanner) getBlockByNumber(number uint) (*BlockDetail, error) {
	blockDetails, err := scanner.getBlockDetailsById(number)
	if err != nil {
		log.Warn("getBlockByNumber", "error", err, "nubmer", number)
		return nil, err
	}
	return blockDetails, nil
}

// get txs
func (scanner *ethSwapScanner) getTransaction(blockDetails *BlockDetail) []*ChunkDetail {
	chunksHash := FilterChunksHash(blockDetails)
	chunksDetail := FilterChunksDetail(chunksHash)
	if len(chunksDetail) == 0 {
		//log.Fatalf("ChunksDetail len is zero")
		return nil
	}
	return chunksDetail
}

func (scanner *ethSwapScanner) getLatestNumber() (*BlockDetail, error) {
	request := &client.Request{}
	request.Method = blockMethod
	request.Params = map[string]string{"finality": "final"}
	request.ID = int(time.Now().UnixNano())
	request.Timeout = rpcTimeout
	var result BlockDetail
	err := client.RPCPostRequest(scanner.gateway, request, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (scanner *ethSwapScanner) getBlockDetailsById(blockId uint) (*BlockDetail, error) {
	request := &client.Request{}
	request.Method = blockMethod
	request.Params = map[string]uint{"block_id": blockId}
	request.ID = int(time.Now().UnixNano())
	request.Timeout = rpcTimeout
	var result BlockDetail
	err := client.RPCPostRequest(scanner.gateway, request, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func GetChunkDetailsByHash(chunkHash string) (*ChunkDetail, error) {
	request := &client.Request{}
	request.Method = chunkMethod
	request.Params = map[string]string{"chunk_id": chunkHash}
	request.ID = int(time.Now().UnixNano())
	request.Timeout = rpcTimeout
	var result ChunkDetail
	err := client.RPCPostRequest(url, request, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func FilterChunksHash(blockDetails *BlockDetail) []string {
	var chunksHash []string
	for _, chunk := range blockDetails.Chunks {
		chunksHash = append(chunksHash, chunk.ChunkHash)
	}
	return chunksHash
}

func FilterChunksDetail(chunksHash []string) []*ChunkDetail {
	var chunksDetail []*ChunkDetail
	for _, chunkHash := range chunksHash {
		chunkDetail, err := GetChunkDetailsByHash(chunkHash)
		if err == nil {
			chunksDetail = append(chunksDetail, chunkDetail)
		}
	}
	return chunksDetail
}

func (scanner *ethSwapScanner) scanTransaction(chunkDetail *ChunkDetail) {
	transactions := chunkDetail.Transactions
	for _, transactionDetail := range transactions {
		if verifyTransaction(transactionDetail) {
			for _, tokenCfg := range params.GetScanConfig().Tokens {
				txHash := transactionDetail.Hash
				scanner.postRouterSwap(txHash, 0, tokenCfg)
			}
		}
	}
}

func verifyTransaction(transactionDetail TransactionDetail) bool {
	if transactionDetail.Actions[0].FunctionCall.MethodName == ftTransferMethod && transactionDetail.ReceiverID == CONTRACT_ID {
		res, err := base64.StdEncoding.DecodeString(transactionDetail.Actions[0].FunctionCall.Args)
		if err != nil {
			log.Fatalf("Failed to decode base64: '%v'", err)
		}
		var ftTransferArgs FtTransfer
		err = json.Unmarshal(res, &ftTransferArgs)
		if err != nil {
			log.Fatalf("Failed to Unmarshal: '%v'", err)
		}
		if ftTransferArgs.ReceiverId == MPC_ID {
			return true
		}
	} else if transactionDetail.Actions[0].FunctionCall.MethodName == swapOutMethod && (transactionDetail.ReceiverID == CONTRACT_ID || transactionDetail.ReceiverID == MPC_ID) {
		return true
	}
	return false
}

type BlockDetail struct {
	Chunks []Chunk `json:"chunks"`
	Header headerConfig `json:"header"`
}

type Chunk struct {
	ChunkHash string `json:"chunk_hash"`
}

type headerConfig struct {
	Hash string `json:"hash"`
	Height uint64 `json:"height"`
}

type ChunkDetail struct {
	Transactions []TransactionDetail `json:"transactions"`
}

type TransactionDetail struct {
	Actions    []Action `json:"actions"`
	Hash       string   `json:"hash"`
	ReceiverID string   `json:"receiver_id"`
}

type Action struct {
	FunctionCall FunctionCall `json:"FunctionCall"`
}

type FunctionCall struct {
	Args       string `json:"args"`
	MethodName string `json:"method_name"`
}

type FtTransfer struct {
	ReceiverId string `json:"receiver_id"`
	Amount     string `json:"amount"`
	Memo       string `json:"memo"`
}

func (scanner *ethSwapScanner) getChainID() (string, error) {
	request := &client.Request{}
	request.Method = genesisMethod
	request.ID = int(time.Now().UnixNano())
	request.Timeout = rpcTimeout
	var result genesisConfig
	err := client.RPCPostRequest(scanner.gateway, request, &result)
	if err != nil {
		return "", err
	}
	return result.ChainID, nil
}

type genesisConfig struct {
	ChainID string `json:"chain_id"`
}


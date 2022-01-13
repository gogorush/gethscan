package params

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/anyswap/CrossChain-Bridge/common"
	"github.com/anyswap/CrossChain-Bridge/log"
)

const TEST bool = false

// swap tx types
const (
	TxSwapin            = "swapin"
	TxSwapout           = "swapout"
	TxSwapout2          = "swapout2" // swapout to string address (eg. BTC)
	TxRouterERC20Swap   = "routerswap"
	TxRouterNFTSwap     = "nftswap"
	TxRouterAnycallSwap = "anycallswap"
)

var (
	configFile       string
	scanConfig       = &ScanConfig{}
	mongodbConfig    = &MongoDBConfig{}
	blockchainConfig = &BlockChainConfig{}
	registerConfig   = &RegisterConfig{}
	approveConfig    = &ApproveConfig{}
)

type Config struct {
	MongoDB    *MongoDBConfig
	BlockChain *BlockChainConfig
	Register   *RegisterConfig
	Tokens     []*TokenConfig
	Approve    *ApproveConfig
}

type ApproveConfig struct {
	TokenAddress string
	LogAddress2 []string
}

// MongoDBConfig mongodb config
type MongoDBConfig struct {
	DBURL    string
	DBName   string
	UserName string `json:"-"`
	Password string `json:"-"`
	Enable   bool
}

type BlockChainConfig struct {
	Chain        string
	StableHeight uint64
	GetLogsMaxBlocks uint64
	GetLogsInterval uint64
	SyncNumber   uint64
}

type RegisterConfig struct {
	Rpc string
	Enable   bool
}

// ScanConfig scan config
type ScanConfig struct {
	Tokens []*TokenConfig
}

// TokenConfig token config
type TokenConfig struct {
	// common
	TxType         string
	SwapServer     string
	CallByContract string   `toml:",omitempty" json:",omitempty"`
	Whitelist      []string `toml:",omitempty" json:",omitempty"`

	// bridge
	PairID         string `toml:",omitempty" json:",omitempty"`
	TokenAddress   string `toml:",omitempty" json:",omitempty"`
	DepositAddress string `toml:",omitempty" json:",omitempty"`

	// router
	ChainID        string `toml:",omitempty" json:",omitempty"`
	RouterContract string `toml:",omitempty" json:",omitempty"`
}

// GetApproveConfig get approve config
func GetApproveConfig() *ApproveConfig {
	return approveConfig
}

// GetMongodbConfig get mongodb config
func GetMongodbConfig() *MongoDBConfig {
	return mongodbConfig
}

// GetBlockChainConfig get blockchain config
func GetBlockChainConfig() *BlockChainConfig {
	return blockchainConfig
}

// GetRegisterConfig get register config
func GetRegisterConfig() *RegisterConfig {
	return registerConfig
}

// IsNativeToken is native token
func (c *TokenConfig) IsNativeToken() bool {
	return c.TokenAddress == "native"
}

// GetScanConfig get scan config
func GetScanConfig() *ScanConfig {
	return scanConfig
}

// LoadConfig load config
func LoadConfig(filePath string) *ScanConfig {
	log.Println("LoadConfig Config file is", filePath)
	if !common.FileExist(filePath) {
		log.Fatalf("LoadConfig error: config file '%v' not exist", filePath)
	}

	config := &Config{}
	if _, err := toml.DecodeFile(filePath, &config); err != nil {
		log.Fatalf("LoadConfig error (toml DecodeFile): %v", err)
	}

	if TEST {
		var bs []byte
		if log.JSONFormat {
			bs, _ = json.Marshal(config)
		} else {
			bs, _ = json.MarshalIndent(config, "", "  ")
		}
		log.Println("LoadConfig finished.", string(bs))
	}

	mongodbConfig = config.MongoDB
	blockchainConfig = config.BlockChain
	registerConfig = config.Register
	scanConfig.Tokens = config.Tokens
	approveConfig = config.Approve

	//if err := scanConfig.CheckConfig(); err != nil {
	//	log.Fatalf("LoadConfig Check config failed. %v", err)
	//}

	configFile = filePath // init config file path
	return scanConfig
}

// ReloadConfig reload config
func ReloadConfig() {
	log.Println("ReloadConfig Config file is", configFile)
	if !common.FileExist(configFile) {
		log.Errorf("ReloadConfig error: config file '%v' not exist", configFile)
		return
	}

	config := &Config{}
	if _, err := toml.DecodeFile(configFile, &config); err != nil {
		log.Errorf("ReloadConfig error (toml DecodeFile): %v", err)
		return
	}

	scanConfig.Tokens = config.Tokens
	if err := scanConfig.CheckConfig(); err != nil {
		log.Errorf("ReloadConfig Check config failed. %v", err)
		return
	}
	log.Println("ReloadConfig success.")
}

// CheckConfig check scan config
func (c *ScanConfig) CheckConfig() (err error) {
	if len(c.Tokens) == 0 {
		log.Warn("CheckConfig", "error", "no token config exist")
		return nil
	}
	pairIDMap := make(map[string]struct{})
	tokensMap := make(map[string]struct{})
	routerswapMap := make(map[string]struct{})
	exist := false
	for _, tokenCfg := range c.Tokens {
		err = tokenCfg.CheckConfig()
		if err != nil {
			return err
		}
		if tokenCfg.IsRouterSwap() || tokenCfg.IsRouterNFTSwap() || tokenCfg.IsRouterAnycallSwap() {
			rkey := strings.ToLower(fmt.Sprintf("%v:%v:%v", tokenCfg.ChainID, tokenCfg.RouterContract, tokenCfg.SwapServer))
			if _, exist = routerswapMap[rkey]; exist {
				return errors.New(fmt.Sprintf("duplicate router swap config tokenCfg.RouterContract: %v", tokenCfg.RouterContract))
			}
			continue
		}
		if tokenCfg.CallByContract != "" {
			continue
		}
		pairIDKey := strings.ToLower(fmt.Sprintf("%v:%v:%v:%v", tokenCfg.TokenAddress, tokenCfg.PairID, tokenCfg.TxType, tokenCfg.SwapServer))
		if _, exist = pairIDMap[pairIDKey]; exist {
			return errors.New(fmt.Sprintf("duplicate pairID config pairIDKey: %v", pairIDKey))
		}
		pairIDMap[pairIDKey] = struct{}{}
		if !tokenCfg.IsNativeToken() {
			tokensKey := strings.ToLower(fmt.Sprintf("%v:%v", tokenCfg.TokenAddress, tokenCfg.DepositAddress))
			if _, exist = tokensMap[tokensKey]; exist {
				return errors.New(fmt.Sprintf("duplicate token config tokensKey: %v, tokenCfg: %v", tokensKey, tokenCfg))
			}
			tokensMap[tokensKey] = struct{}{}
		}
	}
	return nil
}

// IsValidSwapType is valid swap type
func (c *TokenConfig) IsValidSwapType() bool {
	switch c.TxType {
	case
		TxSwapin,
		TxSwapout,
		TxSwapout2,
		TxRouterERC20Swap,
		TxRouterNFTSwap,
		TxRouterAnycallSwap:
		return true
	default:
		return false
	}
}

// IsBridgeSwap is bridge swap
func (c *TokenConfig) IsBridgeSwap() bool {
	switch c.TxType {
	case TxSwapin, TxSwapout, TxSwapout2:
		return true
	default:
		return false
	}
}

// IsBridgeSwapin is bridge swapin
func (c *TokenConfig) IsBridgeSwapin() bool {
	switch c.TxType {
	case TxSwapin:
		return true
	default:
		return false
	}
}

// IsBridgeSwapout is bridge swapout
func (c *TokenConfig) IsBridgeSwapout() bool {
	switch c.TxType {
	case TxSwapout, TxSwapout2:
		return true
	default:
		return false
	}
}

// IsRouterSwap is router swap
func (c *TokenConfig) IsRouterSwap() bool {
	switch c.TxType {
	case TxRouterERC20Swap:
		return true
	default:
		return false
	}
}

// IsRouterAnycallSwap is router anycall swap
func (c *TokenConfig) IsRouterAnycallSwap() bool {
	switch c.TxType {
	case TxRouterAnycallSwap:
		return true
	default:
		return false
	}
}

// IsRouterERC20Swap is router erc20 swap
func (c *TokenConfig) IsRouterERC20Swap() bool {
	return c.TxType == TxRouterERC20Swap
}

// IsRouterNFTSwap is router nft swap
func (c *TokenConfig) IsRouterNFTSwap() bool {
	return c.TxType == TxRouterNFTSwap
}

// CheckConfig check token config
func (c *TokenConfig) CheckConfig() error {
	if !c.IsValidSwapType() {
		return errors.New("invalid 'TxType' " + c.TxType)
	}
	if c.SwapServer == "" {
		return errors.New("empty 'SwapServer'")
	}
	if c.CallByContract != "" && !common.IsHexAddress(c.CallByContract) {
		return errors.New("wrong 'CallByContract' " + c.CallByContract)
	}
	for _, addr := range c.Whitelist {
		if !common.IsHexAddress(addr) {
			return errors.New("wrong 'Whitelist' address " + addr)
		}
	}
	switch {
	case c.IsBridgeSwap():
		if c.PairID == "" {
			return errors.New("empty 'PairID'")
		}
		if c.TxType == TxSwapin && c.CallByContract != "" && c.TokenAddress == "" {
			c.TokenAddress = c.CallByContract // assign token address for swapin if empty
		}
		if !c.IsNativeToken() && !common.IsHexAddress(c.TokenAddress) {
			return errors.New("wrong 'TokenAddress' " + c.TokenAddress)
		}
		if c.DepositAddress != "" && !common.IsHexAddress(c.DepositAddress) {
			return errors.New("wrong 'DepositAddress' " + c.DepositAddress)
		}
	case c.IsRouterSwap():
		if !common.IsHexAddress(c.RouterContract) {
			return errors.New("wrong 'RouterContract' " + c.RouterContract)
		}
		if _, err := common.GetBigIntFromStr(c.ChainID); err != nil {
			return fmt.Errorf("wrong chainID '%v', %w", c.ChainID, err)
		}
	}
	return nil
}

package scanner

import (
	"math/big"

	"github.com/anyswap/CrossChain-Bridge/log"
)

const (
	mainnetNetWork = "mainnet"
	testnetNetWork = "testnet"
)

var (
	// StubChainIDBase stub chainID base value
        StubChainIDBase = big.NewInt(1000000000000)
)

// GetStubChainID get stub chainID
func GetStubChainID(network string) *big.Int {
	stubChainID := new(big.Int).SetBytes([]byte("NEAR"))
	switch network {
	case mainnetNetWork:
	case testnetNetWork:
		stubChainID.Add(stubChainID, big.NewInt(1))
	default:
		log.Fatalf("unknown network %v", network)
	}
	stubChainID.Mod(stubChainID, StubChainIDBase)
	stubChainID.Add(stubChainID, StubChainIDBase)
	return stubChainID
}


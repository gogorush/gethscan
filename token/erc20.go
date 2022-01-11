package token

import (
	"context"
        "encoding/hex"
	"errors"
        "fmt"
	"math"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum"
	com "github.com/weijun-sh/balance-bridge/common"
)

var erc20CodeParts = map[string][]byte{
	"name":      common.FromHex("0x06fdde03"),
	"symbol":    common.FromHex("0x95d89b41"),
	"decimal":   common.FromHex("0x313ce567"),
	"balanceOf": common.FromHex("0x70a08231"),
}

// GetErc20Balance get erc20 decimal balacne of address
func GetErc20Decimal(client *ethclient.Client, contract string) (*big.Int, error) {
	data := make([]byte, 4)
	copy(data[:4], erc20CodeParts["decimal"])
	to := common.HexToAddress(contract)
	msg := ethereum.CallMsg{
		To:   &to,
		Data: data,
	}
	result, err := client.CallContract(context.Background(), msg, nil)
	if err != nil {
		return nil, err
	}
	b := fmt.Sprintf("0x%v", hex.EncodeToString(result))
	return com.GetBigIntFromStr(b)
}

var Decimal map[string]*big.Float = make(map[string]*big.Float)

// GetErc20Balance get erc20 balacne of address
func GetErc20Balance(client *ethclient.Client, contract, address string) (*big.Float, error) {
        //fmt.Printf("getErc20Balance, contract: %v, address: %v\n", contract, address)
        data := make([]byte, 36)
        copy(data[:4], erc20CodeParts["balanceOf"])
        copy(data[4:], common.HexToAddress(address).Hash().Bytes())
        to := common.HexToAddress(contract)
        msg := ethereum.CallMsg{
                To:   &to,
                Data: data,
        }
        result, err := callContract(client, msg)
        if err != nil {
                return big.NewFloat(0), err
        }
        b := fmt.Sprintf("0x%v", hex.EncodeToString(result))
        ret, _ := com.GetBigIntFromStr(b)
        retf := new(big.Float).SetInt(ret)
        decimal, errd := getTokenDecimal(client, contract)
        if errd != nil {
                return big.NewFloat(0), errd
        }
        retf = new(big.Float).Quo(retf, decimal)
        //fmt.Printf("getErc20Balance, retb: %v\n", retf) 
        return retf, nil
}

var getTokenCount int = 3

func callContract(client *ethclient.Client, msg ethereum.CallMsg) ([]byte, error) {
        var i int
        var result []byte
        if client == nil {
                fmt.Printf("callContract, client is nil.\n")
                return []byte{}, errors.New("ErrNoValueObtaind")
        }
        var err error
        for i = 0; i < getTokenCount; i++ {
                result, err = client.CallContract(context.Background(), msg, nil)
                if err == nil {
                        return result, nil
                }
        }
        return []byte{}, errors.New("ErrNoValueObtaind")
}

// getTokenDecimal get token decimal
func getTokenDecimal(client *ethclient.Client, contract string) (*big.Float, error) {
	if Decimal[contract] != nil {
		return Decimal[contract], nil
	}
        //fmt.Printf("getTokenDecimal, contract: %v\n", contract)
        data := make([]byte, 4)
        copy(data[:4], erc20CodeParts["decimal"])
        to := common.HexToAddress(contract)
        msg := ethereum.CallMsg{
                To:   &to,
                Data: data,
        }
        result, err := callContract(client, msg)
        if err != nil {
                return big.NewFloat(0), err
        }
        b := fmt.Sprintf("0x%v", hex.EncodeToString(result))
        decimal, _ := com.GetBigIntFromStr(b)
        Decimal[contract] = big.NewFloat(math.Pow(10, float64(decimal.Int64())))
        //fmt.Printf("getTokenDecimal, retb: %v\n", Decimal)
        return Decimal[contract], nil
}

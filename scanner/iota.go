package scanner

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/anyswap/CrossChain-Bridge/log"
	"github.com/weijun-sh/gethscan/params"
	iotago "github.com/iotaledger/iota.go/v2"

	//"github.com/davecgh/go-spew/spew"
)

var (
	messagesUrl = "api/v1/messages"
	Index = "swapOut"
	//mpc   = "9fb648524b9747608791dbd76bacbebc2f7ac0e3ace10e896739a0a44190102f"
	//mpc   = "atoi1qry7cd950wjuchaq3nup0ltvlh2ay24vmhmc3rw99udg822ux8uycq2x28h"
)

var (
	ErrPayloadType      = errors.New("payload type error")
	ErrTxWithWrongValue = errors.New("tx with wrong value")
	ErrRPCQueryError    = errors.New("rpc query error")
	ErrTxIsNotValidated = errors.New("tx is not validated")
)

type messagesConfig struct {
	Data messagesDataConfig
}

type messagesDataConfig struct {
	Index string
	MaxResults uint64
	Count uint64
	MessageIds []string
}

func (scanner *ethSwapScanner) getMessages_iota() ([]string, error) {
        //fmt.Printf("getBalance4XRP, url: %v, address: %v\n", url, address)
        basket := messagesConfig{}
	urlIndex := fmt.Sprintf("%v/%v?index=%x", scanner.gateway, messagesUrl, []byte(Index))
        for i := 0; i < 1; i++ {
                resp, err := http.Get(urlIndex)
                if err != nil {
                        fmt.Println(err.Error())
                        return nil, err
                }
                defer resp.Body.Close()

                //spew.Printf("resp.Body: %#v\n", resp.Body)
                body, err := ioutil.ReadAll(resp.Body)
                //spew.Printf("body: %#v, string: %v\n", body, string(body))

                if err != nil {
                        fmt.Println(err.Error())
                        return nil, err
                }
                err = json.Unmarshal(body, &basket)
                if err != nil {
                        fmt.Println(err)
                        return nil, err
                }
                //fmt.Printf("%v basket.Result: %v\n", i, basket.Result)
                if basket.Data.Count > 0 {
			log.Info("getMessages_iota", "count", basket.Data.Count)
                        return basket.Data.MessageIds, nil
                } else {
			return nil, errors.New("getMessages_iota null")
		}
        }
        return nil, errors.New("getMessages_iota null")
}

func (scanner *ethSwapScanner) scanMessageId(messageId string) (bool) {
	log.Info("scanMessageId", "messageId", messageId)
	msgID, err := ConvertMessageID(messageId)
	if err != nil {
		log.Debug("ConvertMessageID error", "messageId", messageId, "err", err)
		return false
	}
	_, err = GetTransactionMetadata(scanner.gateway, msgID)
	if err != nil {
		log.Debug("GetTransactionMetadata error", "messageId", messageId, "err", err)
		return false
	}
	txRes, errr := GetTransactionByHash(scanner.gateway, msgID)
	if errr != nil {
		log.Debug("GetTransactionByHash error", "messageId", messageId, "err", errr)
		return false
	}
	payloadRaw, errp := txRes.Payload.MarshalJSON()
	if errp != nil {
		log.Debug("MarshalJSON error", "messageId", messageId, "err", err)
		return false
	}
	ret := true
	for _, tokenCfg := range params.GetScanConfig().Tokens {
		ok := scanner.verifyMessage(messageId, payloadRaw, tokenCfg)
		if ok {
			log.Info("verify success", "messageId", messageId)
			return ret
		}
	}
	return ret
}

func  (scanner *ethSwapScanner) verifyMessage(messageId string, payloadRaw []byte, tokenCfg *params.TokenConfig) bool {
	switch {
	case tokenCfg.IsRouterSwapAll():
		err := ParseMessagePayload(payloadRaw, tokenCfg)
		if err == nil {
			log.Info("verifyMessage", "postRouterSwap", messageId)
			scanner.postRouterSwap(messageId, 0, tokenCfg)
			return true
		}
	default:
		log.Warn("verifyMessage not support", "Chainid", tokenCfg.ChainID)
	}
	return false
}

type Essence struct {
	Type    uint64   `json:"type"`
	Inputs  []Input  `json:"inputs"`
	Outputs []Output `json:"outputs"`
	Payload Payload  `json:"payload"`
}

type Payload struct {
	Type  uint64 `json:"type"`
	Index string `json:"index"`
	Data  string `json:"data"`
}

type Input struct {
	Type                   uint64 `json:"type"`
	TransactionId          string `json:"transactionId"`
	TransactionOutputIndex uint64 `json:"transactionOutputIndex"`
}

type Address struct {
	Type    uint64 `json:"type"`
	Address string `json:"address"`
}

type Output struct {
	Type    uint64  `json:"type"`
	Address Address `json:"address"`
	Amount  uint64  `json:"amount"`
}

type MessagePayload struct {
	Type    uint64  `json:"type"`
	Essence Essence `json:"essence"`
}

func ParseMessagePayload(payload []byte, tokenCfg *params.TokenConfig) error {
	var messagePayload MessagePayload
	if err := json.Unmarshal(payload, &messagePayload); err != nil {
		return err
	} else {
		var amount uint64
		if messagePayload.Type != 0 {
			return ErrPayloadType
		}
		for _, output := range messagePayload.Essence.Outputs {
			log.Info("ParseMessagePayload", "output.Address.Address", output.Address.Address, "tokenCfg.RouterContract", tokenCfg.RouterContract)
			if output.Address.Address == tokenCfg.RouterContract {
				amount += output.Amount
			}
		}
		if amount == 0 {
			return ErrTxWithWrongValue
		} else {
			if err := ParseIndexPayload(messagePayload.Essence.Payload); err != nil {
				return err
			} else {
				return nil
			}
		}
	}
}

func ParseIndexPayload(payload Payload) error {
	if payload.Type != 2 {
		return ErrPayloadType
	}
	if index, err := hex.DecodeString(payload.Index); err != nil || string(index) != Index {
		return err
	}
	if data, err := hex.DecodeString(payload.Data); err != nil {
		return ErrPayloadType
	} else {
		if fields := strings.Split(string(data), ":"); len(fields) != 2 {
			return ErrPayloadType
		} else {
			return nil
		}
	}
}

func ConvertMessageID(txHash string) ([32]byte, error) {
	var msgID [32]byte
	if messageID, err := hex.DecodeString(txHash); err != nil {
		log.Warn("decode message id error", "err", err)
		return msgID, err
	} else {
		copy(msgID[:], messageID)
		return msgID, nil
	}
}

var (
	ctx = context.Background()
)

func GetTransactionMetadata(url string, msgID [32]byte) (*iotago.MessageMetadataResponse, error) {
	nodeHTTPAPIClient := iotago.NewNodeHTTPAPIClient(url)
	if metadataResponse, err := nodeHTTPAPIClient.MessageMetadataByMessageID(ctx, msgID); err != nil {
		return nil, err
	} else {
		if metadataResponse == nil || metadataResponse.LedgerInclusionState == nil {
			return nil, ErrRPCQueryError
		} else if *metadataResponse.LedgerInclusionState != "included" {
			return nil, ErrTxIsNotValidated
		} else {
			return metadataResponse, nil
		}
	}
}

func GetTransactionByHash(url string, msgID [32]byte) (*iotago.Message, error) {
	nodeHTTPAPIClient := iotago.NewNodeHTTPAPIClient(url)
	if messageRes, err := nodeHTTPAPIClient.MessageByMessageID(ctx, msgID); err != nil {
		return nil, err
	} else {
		return messageRes, nil
	}
}

func GetLatestBlockNumber(url string) (uint64, error) {
	nodeHTTPAPIClient := iotago.NewNodeHTTPAPIClient(url)
	if nodeInfoResponse, err := nodeHTTPAPIClient.Info(ctx); err != nil {
		return 0, err
	} else {
		return uint64(nodeInfoResponse.ConfirmedMilestoneIndex), nil
	}
}


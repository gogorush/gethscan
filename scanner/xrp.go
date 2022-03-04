package scanner

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	//"net/url"
	"strings"
	//"unsafe"

	//"github.com/davecgh/go-spew/spew"
)

var URL_xrp string = "https://s1.ripple.com:51234"

func isXRP(chainid string) bool {
	return strings.EqualFold(chainid, "xrp")
}

type latestBlockConfig struct {
	Result latestBlockResultConfig `json:"result"`
}

type latestBlockResultConfig struct {
	Ledger_current_index uint64 `json:"ledger_current_index"`
	Status string `json:"status"`
}

func GetLatestBlockNumber_XRP(url string) (uint64, error) {
	//fmt.Printf("getBalance4XRP, url: %v, address: %v\n", url, address)
	data := "{\"method\":\"ledger_current\",\"params\":[{}]}"
	basket := latestBlockConfig{}
	for i := 0; i < 1; i++ {
		reader := bytes.NewReader([]byte(data))
		resp, err := http.Post(url, "application/json", reader)
		if err != nil {
			fmt.Println(err.Error())
			return 0, err
		}
		defer resp.Body.Close()

		//spew.Printf("resp.Body: %#v\n", resp.Body)
		body, err := ioutil.ReadAll(resp.Body)
		//spew.Printf("body: %#v, string: %v\n", body, string(body))

		if err != nil {
			fmt.Println(err.Error())
			return 0, err
		}
		err = json.Unmarshal(body, &basket)
		if err != nil {
			fmt.Println(err)
			return 0, err
		}
		//fmt.Printf("%v basket.Result: %v\n", i, basket.Result)
		if basket.Result.Status == "success" {
			return basket.Result.Ledger_current_index, nil
		}
	}
	return 0, errors.New("GetLatestBlockNumber_XRP null")
}

type blockTxsConfig struct {
	Result blockTxsResultConfig `json:"result"`
}

type blockTxsResultConfig struct {
	Ledger ledgerConfig `json:"ledger"`
	Status string `json:"status"`
}

type ledgerConfig struct {
	Hash string `json:"hash"`
	Ledger_index string `json:"ledger_index"`
	Transactions []string `json:"transactions"`
}

func GetBlock_XRP(url string, height uint64) (*ledgerConfig, error) {
	//fmt.Printf("getBalance4XRP, url: %v, address: %v\n", url, address)
	data := "{\"method\":\"ledger\",\"params\":[{\"ledger_index\":\""+fmt.Sprintf("%v", height)+"\",\"accounts\":false,\"full\":false,\"transactions\":true,\"expand\":false,\"owner_funds\":false}]}"
	basket := blockTxsConfig{}
	for i := 0; i < 1; i++ {
		reader := bytes.NewReader([]byte(data))
		resp, err := http.Post(url, "application/json", reader)
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
		if basket.Result.Status == "success" {
			return &(basket.Result.Ledger), nil
		}
	}
	return nil, errors.New("GetBlockTxs_XRP null")
}

type txConfig struct {
	Result txResultConfig `json:"result"`
}

type txResultConfig struct {
	Account string `json:"Account"`
	Destination string `json:"Destination"`
	Hash string `json:"hash"`
	Status string `json:"status"`
}

func GetTx_XRP(url, txhash string) (*txResultConfig, error) {
	//fmt.Printf("getBalance4XRP, url: %v, address: %v\n", url, address)
	data := "{\"method\":\"tx\",\"params\":[{\"transaction\":\""+txhash+"\",\"binary\":false}]}"
	basket := txConfig{}
	for i := 0; i < 1; i++ {
		reader := bytes.NewReader([]byte(data))
		resp, err := http.Post(url, "application/json", reader)
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
		if basket.Result.Status == "success" {
			return &basket.Result, nil
		}
	}
	return nil, errors.New("GetBlockTxs_XRP null")
}


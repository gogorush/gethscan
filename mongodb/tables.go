package mongodb

import (
//"gopkg.in/mgo.v2/bson"
)

const (
	tbSwap              string = "scanSwap"
	tbSwapPending       string = "scanPending"
	tbSwapRouterPending string = "scanRouterPending"
	tbSwapDeleted       string = "scanDeleted"
	tbSyncedBlock       string = "syncedBlock"
)

type MgoSwap struct {
	Id         string `bson:"_id"`       //txid
	PairID     string `bson:"pairID"`    //"FXSv4"
	RpcMethod  string `bson:"rpcMethod"` //"swap.Swapin"
	SwapServer string `bson:"swapServer"`
	Chain      string `bson:"chain"`
	ChainID    string `bson:"chainid"`
	LogIndex   string `bson:"logIndex"`
	Timestamp  uint64 `bson:"timestamp"`
}

type MgoSwapRouter struct {
	Id         string `bson:"_id"`       //txid
	LogIndex   string `bson:"logIndex"`  //"FXSv4"
	ChainID    string `bson:"chainID"`   //"FXSv4"
	RpcMethod  string `bson:"rpcMethod"` //"swap.SwapRouter"
	SwapServer string `bson:"swapServer"`
	Chain      string `bson:"chain"`
	Timestamp  uint64 `bson:"timestamp"`
}

type SyncedBlock struct {
	Id          string `bson:"_id"` //"chain"
	Chain       string `bson:"chain"`
	BlockNumber uint64 `bson:"blocknumber"`
}
